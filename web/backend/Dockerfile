FROM node:18-slim

# 安装 openssl，主要用于 Reality 密钥对生成或一些基础网络诊断
RUN apt-get update && apt-get install -y --no-install-recommends \
    openssl \
    curl \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY package*.json ./

RUN npm install --omit=dev

COPY . .

# 暴露面板管理端口
EXPOSE 3000

CMD ["npm", "start"]
