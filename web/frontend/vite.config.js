import { defineConfig } from 'vite';
import vue from '@vitejs/plugin-vue';
import path from 'path';

export default defineConfig({
  plugins: [vue()],
  build: {
    // 将编译生成的文件直接输出到后端服务的静态文件夹中，简化部署步骤
    outDir: path.resolve(__dirname, '../backend/dist'),
    emptyOutDir: true,
  },
  server: {
    host: '0.0.0.0',
    port: 5173,
    // 配置反向代理，方便本地前端独立开发调试
    proxy: {
      '/api': {
        target: 'http://localhost:3000',
        changeOrigin: true,
      }
    }
  }
});
