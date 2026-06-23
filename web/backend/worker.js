const selfHealing = require('./services/self-healing');

console.log('[*] worker process starting');
selfHealing.start();

process.on('SIGINT', () => process.exit(0));
process.on('SIGTERM', () => process.exit(0));
