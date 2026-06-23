#!/bin/bash

# 出口容器启动与网络自愈脚本
# 中文日志输出，方便诊断

set -e

echo "[*] 正在初始化网络策略路由..."

# 1. 备份原默认路由和网卡，用于维持入站回程路由
ORIG_GW=$(ip route show default | awk '{print $3}')
ORIG_DEV=$(ip route show default | awk '{print $5}')

if [ -n "$ORIG_GW" ] && [ -n "$ORIG_DEV" ]; then
    ORIG_IP=$(ip addr show dev "$ORIG_DEV" | grep "inet " | awk '{print $2}' | cut -d/ -f1)
    echo "[+] 检测到原默认网卡: $ORIG_DEV, 原网关: $ORIG_GW, 容器IP: $ORIG_IP"
    
    # 创建策略路由表 100，确保从原网卡进入的流量其回包依然从原网卡送出
    # 解决 OpenVPN 默认路由覆盖导致 VLESS 入站连接无法建立的问题
    ip route add default via "$ORIG_GW" dev "$ORIG_DEV" table 100 || true
    ip rule add from "$ORIG_IP" table 100 || true
    echo "[+] 策略路由配置完成，入站回程链路已锁定至 $ORIG_DEV"
else
    echo "[!] 警告: 未能获取原默认路由，入站连接可能会受到 OpenVPN 路由的影响"
fi

# 2. 启动 OpenVPN
echo "[*] 启动 OpenVPN 客户端..."
if [ ! -f /etc/openvpn/client.ovpn ]; then
    echo "[!] 错误: 未找到 OpenVPN 配置文件 /etc/openvpn/client.ovpn"
    exit 1
fi

# 开启 OpenVPN 拨号并放置于后台运行
openvpn --config /etc/openvpn/client.ovpn --daemon openvpn_client

# 3. 等待 TUN 虚拟网卡就位
echo "[*] 等待 VPN 隧道建立 (tun0)..."
TUN_OK=false
for i in {1..40}; do
    if ip link show tun0 >/dev/null 2>&1; then
        echo "[+] tun0 接口已创建成功"
        TUN_OK=true
        sleep 2
        break
    fi
    echo "    等待中... ($i/40)"
    sleep 1
done

if [ "$TUN_OK" = false ]; then
    echo "[!] 错误: VPN 隧道建立超时，请检查 VPNGate 节点是否有效"
    exit 1
fi

# 4. 测试 VPN 链路的连通性
echo "[*] 测试出口网络连通性并获取外网 IP..."
VPN_IP=""
for i in {1..3}; do
    # 通过 tun0 强制路由获取当前外网 IP
    VPN_IP=$(curl -s --interface tun0 --max-time 10 https://ipinfo.io/ip || true)
    if [ -n "$VPN_IP" ]; then
        echo "[+] VPN 出口建立成功！当前代理出口 IP 为: $VPN_IP"
        break
    fi
    echo "    尝试获取出口 IP 失败，重试中... ($i/3)"
    sleep 2
done

if [ -z "$VPN_IP" ]; then
    echo "[!] 错误: 无法通过 VPN 隧道访问外网，OpenVPN 链路不可用"
    exit 1
fi

# 5. 启动 Xray 服务
echo "[*] 启动 Xray-core 代理服务..."
if [ ! -f /etc/xray/config.json ]; then
    echo "[!] 错误: 未找到 Xray 配置文件 /etc/xray/config.json"
    exit 1
fi

/usr/local/xray/xray run -c /etc/xray/config.json &
XRAY_PID=$!

# 6. 后台循环健康检查守护进程
echo "[*] 开启后台健康状态检测与自愈守护..."
FAILED_COUNT=0
while true; do
    # 检查 Xray 进程是否存活
    if ! kill -0 "$XRAY_PID" 2>/dev/null; then
        echo "[!] 检测到 Xray 进程意外退出，容器终止"
        exit 1
    fi
    
    # 检查 OpenVPN 进程是否存活
    if ! pgrep -f openvpn >/dev/null; then
        echo "[!] 检测到 OpenVPN 进程意外退出，容器终止"
        exit 1
    fi

    # 每 20 秒检测一次代理出口网络连通性
    # 使用多源 IP 检测而非 google.com，避免特定域名被屏蔽导致健康探测误判
    if ! curl -s --interface tun0 --max-time 8 https://ipinfo.io/ip >/dev/null 2>&1 && \
       ! curl -s --interface tun0 --max-time 8 https://api.ipify.org >/dev/null 2>&1; then
        # 注意：不能使用 ((...)) 算术运算，因为 set -e 下表达式结果为 0 会被视为失败退出
        FAILED_COUNT=$((FAILED_COUNT + 1))
        echo "[!] 连通性测试失败 ($FAILED_COUNT/3)"
        if [ "$FAILED_COUNT" -ge 3 ]; then
            echo "[!] 连续 3 次连通性测试失败，判定出口链路已损坏，退出容器以触发自动漂移"
            exit 2
        fi
    else
        FAILED_COUNT=0
    fi
    
    sleep 20
done
