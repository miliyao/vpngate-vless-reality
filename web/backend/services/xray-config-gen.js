// 负责根据模板生成各出口容器所需的 Xray 配置文件
// 中文注释，保持代码规范与鲁棒性

const fs = require('fs');
const path = require('path');

// 容器内运行时模板在 /app/config 目录，本地开发时则回退到项目根目录的 config
const TEMPLATE_PATH = fs.existsSync('/app/config/xray-config.template.json')
  ? '/app/config/xray-config.template.json'
  : path.join(__dirname, '..', '..', '..', 'config', 'xray-config.template.json');

/**
 * 生成 Xray 配置文件并写入指定路径
 * @param {Object} params
 * @param {string} params.uuid - 客户端 UUID
 * @param {string} params.privateKey - Reality 私钥
 * @param {string} params.shortId - Reality shortId
 * @param {string} params.destDomain - 混淆目标域名 (如 www.asus.com:443)
 * @param {string} params.serverName - 混淆 SNI 域名 (如 www.asus.com)
 * @param {string} outputPath - 输出 config.json 的绝对路径
 */
function generateConfig({ uuid, privateKey, shortId, destDomain, serverName }, outputPath) {
  try {
    if (!fs.existsSync(TEMPLATE_PATH)) {
      throw new Error(`模板文件不存在: ${TEMPLATE_PATH}`);
    }

    let templateContent = fs.readFileSync(TEMPLATE_PATH, 'utf-8');

    // 替换模板中的占位符
    templateContent = templateContent
      .replace(/\{\{UUID\}\}/g, uuid)
      .replace(/\{\{PRIVATE_KEY\}\}/g, privateKey)
      .replace(/\{\{SHORT_ID\}\}/g, shortId)
      .replace(/\{\{DEST_DOMAIN\}\}/g, destDomain || 'www.amd.com:443')
      .replace(/\{\{SERVER_NAME\}\}/g, serverName || 'www.amd.com');

    // 确保目标目录存在
    const outputDir = path.dirname(outputPath);
    if (!fs.existsSync(outputDir)) {
      fs.mkdirSync(outputDir, { recursive: true });
    }

    // 写入目标文件
    fs.writeFileSync(outputPath, templateContent, 'utf-8');
    console.log(`[+] Xray 配置文件生成成功: ${outputPath}`);
    return true;
  } catch (error) {
    console.error('[-] 生成 Xray 配置文件失败:', error);
    throw error;
  }
}

module.exports = {
  generateConfig
};
