<div align="center">

# Kael - JumpServer Chat AI powered by Open WebUI

基于 [open-webui](https://github.com/open-webui/open-webui)

</div>

## 项目简介

本项目在开源的 Open WebUI 基础上进行集成与深度定制，重点优化界面适配、交互反馈与主题风格，使其更契合现有产品体系。我们保留了 Open WebUI 的核心能力，并针对业务需求扩展了页面布局、导航策略以及常用功能入口，以提升在真实业务场景中的可用性与一致性。

## 快速开始

### 运行前置

- Node.js `>= 18.13.0 <= 22.x.x`
- npm `>= 6.0.0`

推荐使用 [nvm](https://github.com/nvm-sh/nvm) 管理 Node 版本：

```bash
nvm use 20
```

### 安装依赖

```bash
npm install
```

### 本地开发

```bash
npm run dev
```

默认会启动 Vite 开发服务器并监听局域网访问；若需固定端口，可执行：

```bash
npm run dev:5050
```

### 质量检查

- 前端类型检查：`npm run check`
- ESLint & 格式化修复：`npm run lint` / `npm run format`
- 前端单测 (Vitest)：`npm run test:frontend`
- 后端代码质量 (pylint)：`npm run lint:backend`

> 若仅需要自动修复前端 lint，可执行 `npm run lint:frontend`。

### 构建与预览

```bash
npm run build
npm run preview
```

`build` 将生成生产环境静态资源，`preview` 用于本地验证构建产物。

## 项目结构速览

```
├── backend/           # 后端接口与服务
├── src/               # Svelte 前端源码
│   ├── lib/           # 可复用组件、hooks 与工具
│   ├── routes/        # 页面与布局
│   └── app.d.ts       # 类型定义与应用配置
├── static/            # 静态资源
├── svelte.config.js   # SvelteKit 配置
├── package.json       # npm 配置与脚本
└── README.md          # 项目说明
```

> 目录仅示意关键部分，实际结构以仓库为准。

## 定制说明

- **样式主题**：统一由 `src/lib/theme` 及相关 Tailwind 配置管理；可通过 `tailwind.config.js` 调整设计变量。
- **导航与路由**：集中在 `src/routes` 目录，采用 SvelteKit 默认约定；新增页面时请确保更新导航映射。
- **接口对接**：后端接口位于 `backend/`，前端通过 `src/lib/api` 进行封装与调用。
- **国际化**：若启用多语言，请使用 `npm run i18n:parse` 更新词条，并在 `src/lib/i18n` 维护翻译资源。

## 部署建议

1. 执行 `npm run build` 生成静态资源。
2. 根据部署目标选择适配器（默认 `adapter-auto`），或在 `svelte.config.js` 中切换为 `adapter-node`/`adapter-static`。
3. 将 `build/` 目录产物整合进主产品的构建流程中，确保静态资源路径与代理配置一致。
4. 若集成在现有后端中，建议使用反向代理统一鉴权与会话管理。

## 常见问题

- **依赖安装失败**：请确认 Node 与 npm 版本符合要求，可尝试删除 `node_modules` 与 `package-lock.json` 后重新安装。
- **样式未生效**：确认已执行 `npm run dev`，并检查 Tailwind 配置是否被覆盖。
- **构建体积过大**：可通过移除未使用的插件、启用代码分割或调整 `vite.config.ts` 进行优化。

## 贡献指南

1. Fork 仓库并新建分支（推荐 `feature/xxx` 或 `fix/xxx` 命名）。
2. 完成开发后执行 `npm run lint` 与 `npm run test:frontend` 确保质量。
3. 提交 Pull Request，并在描述中说明变更背景与测试情况。

## 许可证

本项目基于 Open WebUI，遵循其上游开源协议。项目内新增或修改的部分，如无特殊声明，默认沿用上游许可。请在合规前提下使用与分发。
