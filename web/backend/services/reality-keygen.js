// 基于 Node.js 原生 crypto 模块生成 X25519 密钥对（Reality 协议专用）
// 避免依赖外部 xray 二进制，实现完全解耦和跨平台运行

const crypto = require('crypto');

function generateRealityKeys() {
  // 生成 X25519 密钥对
  const { privateKey, publicKey } = crypto.generateKeyPairSync('x25519');

  // 导出为 DER 格式
  const privDer = privateKey.export({ format: 'der', type: 'pkcs8' });
  const pubDer = publicKey.export({ format: 'der', type: 'spki' });

  // 提取原始 32 字节的私钥与公钥
  // X25519 PKCS8 的 DER 编码中，头部固定为 16 字节，私钥位于后 32 字节
  // X25519 SPKI 的 DER 编码中，头部固定为 12 字节，公钥位于后 32 字节
  const rawPrivate = privDer.subarray(privDer.length - 32);
  const rawPublic = pubDer.subarray(pubDer.length - 32);

  // Reality 使用 base64url 格式（即 URL 安全的 Base64，不含等号）
  const privateKeyBase64 = rawPrivate.toString('base64url');
  const publicKeyBase64 = rawPublic.toString('base64url');

  // 生成 8 字节（16位十六进制字符）的 shortId
  const shortId = crypto.randomBytes(8).toString('hex');

  return {
    privateKey: privateKeyBase64,
    publicKey: publicKeyBase64,
    shortId: shortId
  };
}

module.exports = {
  generateRealityKeys
};
