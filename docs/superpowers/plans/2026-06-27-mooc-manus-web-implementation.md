# mooc-manus-web 前端项目实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 构建智能体编排管理平台的可视化前端应用,支持模型配置、工具管理、Skill管理、SSE流式对话

**Architecture:** React 18 + TypeScript 5 + Vite 5 + Ant Design 5 + Zustand,按职能分层(api/components/pages/store/types),采用卡片式管理界面和左右分栏对话界面

**Tech Stack:** React, TypeScript, Vite, Ant Design, Zustand, Axios, EventSource, React Router, tailwindcss

**Spec Document:** `docs/superpowers/specs/2026-06-26-mooc-manus-web-design.md`

**Backend Repository:** https://github.com/pw151294/mooc-manus.git

---

## 阶段1: 基础工程初始化

**目标:** 搭建可运行的前端项目骨架,配置开发环境

---

### Task 1.1: 创建Vite + React + TypeScript项目

**Files:**
- Create: `mooc-manus-web/` (项目根目录)

- [ ] **Step 1: 创建项目目录并初始化Vite项目**

```bash
cd /Users/panwei/Downloads/python/mcp+A2A
npm create vite@latest mooc-manus-web -- --template react-ts
```

Expected: 项目创建成功,生成基础文件结构

- [ ] **Step 2: 进入项目目录并安装依赖**

```bash
cd mooc-manus-web
npm install
```

Expected: 依赖安装成功,生成node_modules/和package-lock.json

- [ ] **Step 3: 验证项目可启动**

```bash
npm run dev
```

Expected: 
- 输出: `Local: http://localhost:5173/`
- 浏览器打开可看到Vite + React默认页面

- [ ] **Step 4: 停止开发服务器并提交初始代码**

```bash
# Ctrl+C 停止服务器
git init
git add .
git commit -m "chore: initialize vite + react + typescript project"
```

Expected: Git仓库初始化完成,初始代码已提交

---

### Task 1.2: 安装核心依赖

**Files:**
- Modify: `mooc-manus-web/package.json`

- [ ] **Step 1: 安装Ant Design及相关依赖**

```bash
npm install antd @ant-design/icons
```

Expected: package.json中添加antd和@ant-design/icons依赖

- [ ] **Step 2: 安装路由和状态管理**

```bash
npm install react-router-dom zustand
```

Expected: 安装成功

- [ ] **Step 3: 安装网络请求和工具库**

```bash
npm install axios dayjs lodash-es uuid
npm install -D @types/lodash-es @types/uuid
```

Expected: 安装成功,包含类型定义

- [ ] **Step 4: 安装tailwindcss**

```bash
npm install -D tailwindcss postcss autoprefixer
npx tailwindcss init -p
```

Expected: 生成tailwind.config.js和postcss.config.js

- [ ] **Step 5: 提交依赖变更**

```bash
git add package.json package-lock.json tailwind.config.js postcss.config.js
git commit -m "chore: install core dependencies (antd, router, zustand, axios, etc)"
```

---

### Task 1.3: 配置tailwindcss

**Files:**
- Modify: `mooc-manus-web/tailwind.config.js`
- Modify: `mooc-manus-web/src/index.css`

- [ ] **Step 1: 配置tailwind.config.js**

```javascript
/** @type {import('tailwindcss').Config} */
export default {
  content: [
    "./index.html",
    "./src/**/*.{js,ts,jsx,tsx}",
  ],
  theme: {
    extend: {},
  },
  plugins: [],
}
```

- [ ] **Step 2: 在index.css中引入tailwind指令和Ant Design样式**

```css
@import 'antd/dist/reset.css';

@tailwind base;
@tailwind components;
@tailwind utilities;
```

- [ ] **Step 3: 提交tailwind配置**

```bash
git add tailwind.config.js src/index.css
git commit -m "chore: configure tailwindcss"
```

---

### Task 1.4: 配置ESLint和Prettier

**Files:**
- Create: `mooc-manus-web/.prettierrc`
- Create: `mooc-manus-web/.prettierignore`
- Modify: `mooc-manus-web/.eslintrc.cjs` (if exists) or Create

- [ ] **Step 1: 安装Prettier**

```bash
npm install -D prettier eslint-config-prettier eslint-plugin-prettier
```

- [ ] **Step 2: 创建.prettierrc配置文件**

```json
{
  "semi": true,
  "singleQuote": true,
  "tabWidth": 2,
  "trailingComma": "es5",
  "printWidth": 100
}
```

- [ ] **Step 3: 创建.prettierignore文件**

```
node_modules
dist
build
.vite
*.md
```

- [ ] **Step 4: 配置ESLint集成Prettier**

修改`.eslintrc.cjs`(如果不存在则创建):

```javascript
module.exports = {
  root: true,
  env: { browser: true, es2020: true },
  extends: [
    'eslint:recommended',
    'plugin:@typescript-eslint/recommended',
    'plugin:react-hooks/recommended',
    'prettier', // 必须放在最后
  ],
  ignorePatterns: ['dist', '.eslintrc.cjs'],
  parser: '@typescript-eslint/parser',
  plugins: ['react-refresh', 'prettier'],
  rules: {
    'react-refresh/only-export-components': [
      'warn',
      { allowConstantExport: true },
    ],
    'prettier/prettier': 'error',
    '@typescript-eslint/no-explicit-any': 'warn',
  },
}
```

- [ ] **Step 5: 在package.json中添加格式化脚本**

在`scripts`中添加:

```json
{
  "scripts": {
    "format": "prettier --write \"src/**/*.{ts,tsx,js,jsx,json,css}\"",
    "lint": "eslint . --ext ts,tsx --report-unused-disable-directives --max-warnings 0"
  }
}
```

- [ ] **Step 6: 运行格式化检查**

```bash
npm run format
npm run lint
```

Expected: 代码格式化完成,无ESLint错误

- [ ] **Step 7: 提交配置**

```bash
git add .prettierrc .prettierignore .eslintrc.cjs package.json src/
git commit -m "chore: configure eslint and prettier"
```

---

### Task 1.5: 配置Vite和环境变量

**Files:**
- Modify: `mooc-manus-web/vite.config.ts`
- Create: `mooc-manus-web/.env.development`
- Create: `mooc-manus-web/.env.production`

- [ ] **Step 1: 配置vite.config.ts**

```typescript
import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import path from 'path';

export default defineConfig({
  plugins: [react()],
  server: {
    port: 3000,
    proxy: {
      '/api': {
        target: 'http://localhost:8080',
        changeOrigin: true,
      },
    },
  },
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
    },
  },
});
```

- [ ] **Step 2: 创建.env.development**

```bash
VITE_API_BASE_URL=http://localhost:8080
```

- [ ] **Step 3: 创建.env.production**

```bash
VITE_API_BASE_URL=https://your-production-api.com
```

- [ ] **Step 4: 配置tsconfig.json支持@别名**

在`tsconfig.json`的`compilerOptions`中添加:

```json
{
  "compilerOptions": {
    "baseUrl": ".",
    "paths": {
      "@/*": ["src/*"]
    }
  }
}
```

- [ ] **Step 5: 提交配置**

```bash
git add vite.config.ts .env.development .env.production tsconfig.json
git commit -m "chore: configure vite proxy and environment variables"
```

---

### Task 1.6: 创建基础目录结构

**Files:**
- Create: `mooc-manus-web/src/api/`
- Create: `mooc-manus-web/src/api/modules/`
- Create: `mooc-manus-web/src/components/`
- Create: `mooc-manus-web/src/pages/`
- Create: `mooc-manus-web/src/store/`
- Create: `mooc-manus-web/src/types/`
- Create: `mooc-manus-web/src/utils/`
- Create: `mooc-manus-web/src/constants/`
- Create: `mooc-manus-web/src/hooks/`
- Create: `mooc-manus-web/src/router/`

- [ ] **Step 1: 创建所有目录**

```bash
cd src
mkdir -p api/modules components pages store types utils constants hooks router
cd ..
```

Expected: 目录结构创建成功

- [ ] **Step 2: 在每个目录创建.gitkeep文件(保持空目录)**

```bash
touch src/api/modules/.gitkeep
touch src/components/.gitkeep
touch src/pages/.gitkeep
touch src/store/.gitkeep
touch src/types/.gitkeep
touch src/utils/.gitkeep
touch src/constants/.gitkeep
touch src/hooks/.gitkeep
touch src/router/.gitkeep
```

- [ ] **Step 3: 提交目录结构**

```bash
git add src/
git commit -m "chore: create base directory structure"
```

---

### Task 1.7: 创建基础Layout组件

**Files:**
- Create: `mooc-manus-web/src/components/Layout/index.tsx`
- Create: `mooc-manus-web/src/components/Layout/index.module.css`

- [ ] **Step 1: 创建Layout组件**

```typescript
// src/components/Layout/index.tsx
import React from 'react';
import { Layout as AntLayout, Menu } from 'antd';
import { Outlet, useNavigate, useLocation } from 'react-router-dom';
import {
  MessageOutlined,
  SettingOutlined,
  ToolOutlined,
  ThunderboltOutlined,
} from '@ant-design/icons';

const { Header, Sider, Content } = AntLayout;

const Layout: React.FC = () => {
  const navigate = useNavigate();
  const location = useLocation();

  const menuItems = [
    {
      key: '/agent',
      icon: <MessageOutlined />,
      label: '智能体对话',
    },
    {
      key: '/model-config',
      icon: <SettingOutlined />,
      label: '模型配置',
    },
    {
      key: '/tools',
      icon: <ToolOutlined />,
      label: '工具管理',
      children: [
        { key: '/tools/providers', label: '工具供应商' },
        { key: '/tools/functions', label: '工具函数' },
      ],
    },
    {
      key: '/skills',
      icon: <ThunderboltOutlined />,
      label: 'Skill管理',
    },
  ];

  return (
    <AntLayout style={{ minHeight: '100vh' }}>
      <Header style={{ display: 'flex', alignItems: 'center', padding: '0 24px' }}>
        <div style={{ color: 'white', fontSize: '20px', fontWeight: 'bold' }}>
          Mooc Manus
        </div>
      </Header>
      <AntLayout>
        <Sider width={200} theme="light">
          <Menu
            mode="inline"
            selectedKeys={[location.pathname]}
            defaultOpenKeys={['/tools']}
            items={menuItems}
            onClick={({ key }) => navigate(key)}
            style={{ height: '100%', borderRight: 0 }}
          />
        </Sider>
        <Content style={{ padding: '24px', background: '#f0f2f5' }}>
          <Outlet />
        </Content>
      </AntLayout>
    </AntLayout>
  );
};

export default Layout;
```

- [ ] **Step 2: 提交Layout组件**

```bash
git add src/components/Layout/
git commit -m "feat: create base layout component with sidebar navigation"
```

---

### Task 1.8: 验证阶段1完成

- [ ] **Step 1: 启动开发服务器**

```bash
npm run dev
```

Expected: 
- 服务器在 http://localhost:3000 启动
- 无编译错误和警告

- [ ] **Step 2: 访问应用**

打开浏览器访问 http://localhost:3000

Expected: 可看到应用界面(暂时还是默认页面,下一阶段配置路由后会使用Layout)

- [ ] **Step 3: 检查Git状态**

```bash
git status
```

Expected: 工作目录干净,所有更改已提交

---

**阶段1完成标志:**
- ✅ 项目可成功启动
- ✅ 无编译错误和警告  
- ✅ 基础目录结构已创建
- ✅ Layout组件已创建
- ✅ 所有更改已提交到Git

---

## 阶段2: 全局公共能力封装

**目标:** 完成Axios请求封装、SSE客户端封装、类型定义、路由配置

---

### Task 2.1: 定义SSE事件类型

**Files:**
- Create: `mooc-manus-web/src/types/sse.ts`

- [ ] **Step 1: 创建SSE事件类型定义**

```typescript
// src/types/sse.ts

// SSE事件类型枚举
export type SSEEventType =
  | 'message'
  | 'message_end'
  | 'tool_call_start'
  | 'tool_call_complete'
  | 'tool_call_fail'
  | 'error'
  | 'done'
  | 'title'
  | 'plan_create_success'
  | 'step_start'
  | 'step_complete';

// 基础事件数据
export interface BaseEventData {
  id: string;
  conversationId: string;
  type: string;
  timestamp: string;
}

// 消息事件
export interface MessageEventData extends BaseEventData {
  type: 'message' | 'message_end';
  timestamp: string;
  role: 'user' | 'assistant';
  message: string;
  attachments?: any[];
}

// 工具调用事件
export interface ToolEventData extends BaseEventData {
  type: 'tool_call_start' | 'tool_call_complete' | 'tool_call_fail';
  timestamp: string;
  tool_call_id: string;
  tool_name: string;
  function_name: string;
  function_args: string;
  function_result?: any;
  status: 'calling' | 'completed' | 'failed';
}

// 错误事件
export interface ErrorEventData extends BaseEventData {
  type: 'error';
  timestamp: string;
  error: string;
}

// 完成事件
export interface DoneEventData extends BaseEventData {
  type: 'done';
  timestamp: string;
}

// 标题事件
export interface TitleEventData extends BaseEventData {
  type: 'title';
  timestamp: string;
  title: string;
}

// 联合类型
export type SSEEventData =
  | MessageEventData
  | ToolEventData
  | ErrorEventData
  | DoneEventData
  | TitleEventData;

// SSE事件处理器
export interface SSEHandlers {
  onOpen?: () => void;
  onEvent: (eventType: SSEEventType, data: SSEEventData) => void;
  onError?: (error: any) => void;
  onComplete?: () => void;
}
```

- [ ] **Step 2: 提交SSE类型定义**

```bash
git add src/types/sse.ts
git commit -m "feat: define sse event types"
```

---

### Task 2.2: 封装SSE客户端

**Files:**
- Create: `mooc-manus-web/src/api/sse.ts`

- [ ] **Step 1: 创建SSE客户端类**

```typescript
// src/api/sse.ts
import type { SSEEventType, SSEHandlers } from '@/types/sse';

class SSEClient {
  private eventSource: EventSource | null = null;
  private timeout: number = 60000; // 1分钟超时
  private timeoutTimer: NodeJS.Timeout | null = null;
  private isActive: boolean = false; // 防止重复订阅

  subscribe(url: string, handlers: SSEHandlers): void {
    // 防止重复订阅
    if (this.isActive) {
      throw new Error('SSE连接已存在,请先关闭');
    }

    this.eventSource = new EventSource(url);
    this.isActive = true;
    this.resetTimeout();

    // 连接建立回调
    this.eventSource.addEventListener('open', () => {
      console.log('SSE连接已建立');
      handlers.onOpen?.();
    });

    // 监听所有事件类型
    const eventTypes: SSEEventType[] = [
      'message',
      'message_end',
      'tool_call_start',
      'tool_call_complete',
      'tool_call_fail',
      'error',
      'done',
      'title',
      'plan_create_success',
      'step_start',
      'step_complete',
    ];

    eventTypes.forEach((type) => {
      this.eventSource!.addEventListener(type, (e: MessageEvent) => {
        this.resetTimeout(); // 收到消息重置超时

        // JSON解析容错
        try {
          const data = JSON.parse(e.data);
          handlers.onEvent(type, data);

          // done事件自动关闭
          if (type === 'done') {
            this.close();
            handlers.onComplete?.();
          }
        } catch (err) {
          console.error('SSE数据解析失败:', err, 'raw data:', e.data);
          handlers.onError?.(new Error('数据格式错误'));
        }
      });
    });

    // 错误处理
    this.eventSource.onerror = (error) => {
      console.error('SSE连接错误:', error);
      this.close();
      handlers.onError?.(error);
    };
  }

  private resetTimeout(): void {
    if (this.timeoutTimer) {
      clearTimeout(this.timeoutTimer);
    }
    this.timeoutTimer = setTimeout(() => {
      this.close();
      console.warn('SSE connection timeout');
    }, this.timeout);
  }

  close(): void {
    this.isActive = false;
    if (this.timeoutTimer) {
      clearTimeout(this.timeoutTimer);
      this.timeoutTimer = null;
    }
    if (this.eventSource) {
      this.eventSource.close();
      this.eventSource = null;
    }
  }
}

export default SSEClient;
```

- [ ] **Step 2: 提交SSE客户端**

```bash
git add src/api/sse.ts
git commit -m "feat: implement sse client with timeout and error handling"
```

---

### Task 2.3: 封装Axios请求

**Files:**
- Create: `mooc-manus-web/src/api/request.ts`

- [ ] **Step 1: 创建Axios实例和拦截器**

```typescript
// src/api/request.ts
import axios, { AxiosError, AxiosResponse } from 'axios';
import { message } from 'antd';

// 创建Axios实例
const request = axios.create({
  baseURL: import.meta.env.VITE_API_BASE_URL || 'http://localhost:8080',
  timeout: 30000,
  headers: {
    'Content-Type': 'application/json',
  },
});

// 请求拦截器
request.interceptors.request.use(
  (config) => {
    // 可在此添加token等认证信息
    return config;
  },
  (error) => {
    return Promise.reject(error);
  }
);

// 响应拦截器
request.interceptors.response.use(
  (response: AxiosResponse) => {
    // 后端直接返回数据(无统一封装),直接透传
    return response.data;
  },
  (error: AxiosError<{ error?: string }>) => {
    // 统一错误处理
    if (error.response) {
      const { status, data } = error.response;

      switch (status) {
        case 400:
          message.error(data.error || '请求参数错误');
          break;
        case 404:
          message.error(data.error || '资源不存在');
          break;
        case 409:
          message.error(data.error || '资源冲突');
          break;
        case 500:
          message.error(data.error || '服务器错误');
          break;
        default:
          message.error(data.error || '请求失败');
      }
    } else if (error.request) {
      message.error('网络连接失败');
    } else {
      message.error('请求配置错误');
    }

    return Promise.reject(error);
  }
);

export default request;
```

- [ ] **Step 2: 提交Axios封装**

```bash
git add src/api/request.ts
git commit -m "feat: implement axios request with interceptors and error handling"
```

---

### Task 2.4: 配置React Router

**Files:**
- Create: `mooc-manus-web/src/router/index.tsx`
- Create: `mooc-manus-web/src/pages/Agent/index.tsx`
- Create: `mooc-manus-web/src/pages/AppConfig/index.tsx`
- Create: `mooc-manus-web/src/pages/Tool/Providers.tsx`
- Create: `mooc-manus-web/src/pages/Tool/Functions.tsx`
- Create: `mooc-manus-web/src/pages/Skill/index.tsx`
- Modify: `mooc-manus-web/src/App.tsx`
- Modify: `mooc-manus-web/src/main.tsx`

- [ ] **Step 1: 创建占位页面组件**

```typescript
// src/pages/Agent/index.tsx
import React from 'react';

const AgentPage: React.FC = () => {
  return <div>智能体对话页面</div>;
};

export default AgentPage;
```

```typescript
// src/pages/AppConfig/index.tsx
import React from 'react';

const AppConfigPage: React.FC = () => {
  return <div>模型配置管理页面</div>;
};

export default AppConfigPage;
```

```typescript
// src/pages/Tool/Providers.tsx
import React from 'react';

const ToolProvidersPage: React.FC = () => {
  return <div>工具供应商管理页面</div>;
};

export default ToolProvidersPage;
```

```typescript
// src/pages/Tool/Functions.tsx
import React from 'react';

const ToolFunctionsPage: React.FC = () => {
  return <div>工具函数管理页面</div>;
};

export default ToolFunctionsPage;
```

```typescript
// src/pages/Skill/index.tsx
import React from 'react';

const SkillPage: React.FC = () => {
  return <div>Skill管理页面</div>;
};

export default SkillPage;
```

- [ ] **Step 2: 创建路由配置**

```typescript
// src/router/index.tsx
import { createBrowserRouter, Navigate } from 'react-router-dom';
import Layout from '@/components/Layout';
import AgentPage from '@/pages/Agent';
import AppConfigPage from '@/pages/AppConfig';
import ToolProvidersPage from '@/pages/Tool/Providers';
import ToolFunctionsPage from '@/pages/Tool/Functions';
import SkillPage from '@/pages/Skill';

const router = createBrowserRouter([
  {
    path: '/',
    element: <Layout />,
    children: [
      {
        index: true,
        element: <Navigate to="/agent" replace />,
      },
      {
        path: 'agent',
        element: <AgentPage />,
      },
      {
        path: 'model-config',
        element: <AppConfigPage />,
      },
      {
        path: 'tools/providers',
        element: <ToolProvidersPage />,
      },
      {
        path: 'tools/functions',
        element: <ToolFunctionsPage />,
      },
      {
        path: 'skills',
        element: <SkillPage />,
      },
    ],
  },
]);

export default router;
```

- [ ] **Step 3: 修改main.tsx使用RouterProvider**

```typescript
// src/main.tsx
import React from 'react';
import ReactDOM from 'react-dom/client';
import { RouterProvider } from 'react-router-dom';
import router from './router';
import './index.css';

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <RouterProvider router={router} />
  </React.StrictMode>
);
```

- [ ] **Step 4: 清理App.tsx(不再使用)**

```typescript
// src/App.tsx
// 此文件已不再使用,由router配置替代
export {};
```

- [ ] **Step 5: 提交路由配置**

```bash
git add src/router/ src/pages/ src/main.tsx src/App.tsx
git commit -m "feat: configure react-router with layout and placeholder pages"
```

---

### Task 2.5: 验证阶段2完成

- [ ] **Step 1: 启动开发服务器**

```bash
npm run dev
```

- [ ] **Step 2: 测试路由跳转**

访问 http://localhost:3000

Expected:
- 自动重定向到 /agent
- 左侧菜单可点击,页面切换正常
- 无控制台错误

- [ ] **Step 3: 测试后端连接(可选,需要后端服务启动)**

在浏览器控制台执行:

```javascript
fetch('/api/status').then(r => r.json()).then(console.log)
```

Expected: 如果后端启动,返回健康检查数据;否则返回网络错误(符合预期)

- [ ] **Step 4: 检查Git状态**

```bash
git status
```

Expected: 工作目录干净

---

**阶段2完成标志:**
- ✅ SSE客户端已封装
- ✅ Axios请求已封装
- ✅ 路由配置完成
- ✅ 所有页面可正常访问

---


## 阶段3-7: 业务模块开发(高层次任务概要)

> **说明:** 以下阶段采用高层次任务描述,实际执行时需要根据具体情况细化步骤。每个任务建议遵循相同模式:定义类型→实现API→创建store→开发页面→测试→提交。执行者可参考阶段1-2的详细步骤风格进行细化。

---

## 阶段3: 模型配置管理模块

**目标:** 完成AppConfig模块的完整CRUD,采用卡片式界面

### Task 3.1: 定义AppConfig类型
**Files:** `src/types/appConfig.ts`
**内容:** 定义AppConfigDTO、AppConfigCreateRequest、AppConfigUpdateRequest等类型

### Task 3.2: 实现AppConfig API接口
**Files:** `src/api/modules/appConfig.ts`
**内容:** 实现createAppConfig、updateAppConfig、getAppConfig、listAppConfigs、deleteAppConfig

### Task 3.3: 创建AppConfig状态管理
**Files:** `src/store/appConfig.ts`
**内容:** 使用Zustand创建store,管理列表数据、加载状态、CRUD操作

### Task 3.4: 开发AppConfig卡片列表页面
**Files:** `src/pages/AppConfig/index.tsx`, `src/pages/AppConfig/ConfigCard.tsx`
**内容:** Grid布局卡片列表,每个卡片显示模型名称、BaseURL、温度、MaxTokens等参数

### Task 3.5: 开发新增/编辑Modal表单
**Files:** `src/pages/AppConfig/ConfigForm.tsx`
**内容:** Ant Design Modal + Form,包含所有字段校验(baseUrl必填、temperature范围0-1等)

### Task 3.6: 实现搜索和删除功能
**内容:** 顶部搜索框(关键词过滤)、卡片删除按钮(Popconfirm二次确认)

### Task 3.7: 测试AppConfig模块
**步骤:**
1. 启动后端服务
2. 测试新增AppConfig
3. 测试编辑AppConfig
4. 测试删除AppConfig
5. 测试搜索功能
6. 确认列表自动刷新

### Task 3.8: 提交阶段3代码
```bash
git add src/types/appConfig.ts src/api/modules/appConfig.ts src/store/appConfig.ts src/pages/AppConfig/
git commit -m "feat(appconfig): implement complete crud with card-style ui"
```

---

## 阶段4: 工具管理模块

**目标:** 完成Tool Provider和Tool Function管理,采用卡片式界面

### Task 4.1: 定义Tool类型
**Files:** `src/types/tool.ts`
**内容:** 定义ToolProviderDTO、ToolFunctionDTO、相关Request类型

### Task 4.2: 实现Tool API接口
**Files:** `src/api/modules/tool.ts`
**内容:** Provider和Function的CRUD接口、按Provider查询Function

### Task 4.3: 创建Tool状态管理
**Files:** `src/store/tool.ts`
**内容:** 管理Provider列表、Function列表、关联关系

### Task 4.4: 开发工具供应商管理页面
**Files:** `src/pages/Tool/Providers.tsx`, `src/pages/Tool/ProviderCard.tsx`, `src/pages/Tool/ProviderForm.tsx`
**内容:** 卡片列表 + Modal表单

### Task 4.5: 开发工具函数管理页面
**Files:** `src/pages/Tool/Functions.tsx`, `src/pages/Tool/FunctionCard.tsx`, `src/pages/Tool/FunctionForm.tsx`
**内容:** 
- 顶部Provider选择器(下拉框)
- 卡片列表展示Function
- Modal表单(包含JSON Schema参数配置)

### Task 4.6: 测试Tool模块
**验收:** Provider和Function CRUD正常,按Provider筛选Function正常

### Task 4.7: 提交阶段4代码
```bash
git commit -m "feat(tool): implement provider and function management"
```

---

## 阶段5: Skill管理模块

**目标:** 完成Skill混合式管理界面(左侧Provider树 + 右侧Skill卡片)

### Task 5.1: 定义Skill类型
**Files:** `src/types/skill.ts`
**内容:** SkillDTO、SkillVersionDTO、SkillProviderDTO、相关Request类型

### Task 5.2: 实现Skill API接口
**Files:** `src/api/modules/skill.ts`
**内容:** Skill/Provider/Version/ImportTask的所有接口

### Task 5.3: 创建Skill状态管理
**Files:** `src/store/skill.ts`
**内容:** 管理Provider树状态、Skill列表、当前选中Provider

### Task 5.4: 开发Skill主页面布局
**Files:** `src/pages/Skill/index.tsx`
**内容:** Ant Design Layout,左侧Sider(250px) + 右侧Content

### Task 5.5: 开发Provider树组件
**Files:** `src/pages/Skill/ProviderTree.tsx`
**内容:** Ant Design Tree,支持折叠/展开,点击节点过滤右侧Skill

### Task 5.6: 开发Skill卡片列表
**Files:** `src/pages/Skill/SkillCard.tsx`
**内容:** Grid布局,展示Skill名称、版本、状态、图标

### Task 5.7: 开发Skill详情Modal
**Files:** `src/pages/Skill/SkillDetailModal.tsx`
**内容:** 
- Tabs: 基本信息、版本列表、文件列表
- 版本列表支持切换、回滚、导出
- 文件列表支持下载

### Task 5.8: 开发导入任务页面
**Files:** `src/pages/Skill/ImportTasks.tsx`
**内容:**
- 使用SSE客户端订阅进度
- 实时更新进度条和日志
- Progress + List组件展示

### Task 5.9: 测试Skill模块
**验收:**
- Provider树正常工作
- Skill卡片过滤正常
- 详情Modal展示完整
- 导入任务SSE实时更新

### Task 5.10: 提交阶段5代码
```bash
git commit -m "feat(skill): implement hybrid management ui with provider tree and sse import"
```

---

## 阶段6: 智能体对话模块

**目标:** 完成左右分栏对话界面 + SSE流式对话

### Task 6.1: 定义Agent类型
**Files:** `src/types/agent.ts`
**内容:** Message、ToolCallStatus、ChatRequest等类型

### Task 6.2: 实现Agent API接口
**Files:** `src/api/modules/agent.ts`
**内容:** 组装SSE URL,调用SSE客户端

### Task 6.3: 创建Agent状态管理
**Files:** `src/store/agent.ts`
**内容:** 
- 能力装配状态(selectedConfig, selectedTools, selectedSkills)
- 对话状态(messages, conversationId, isStreaming)
- Actions(addMessage, updateLastMessage, addToolCallStatus等)

### Task 6.4: 开发左侧配置面板
**Files:** `src/pages/Agent/ConfigPanel.tsx`
**内容:**
- 模型选择下拉框(从AppConfig列表加载)
- 工具包复选框(从Tool Function加载)
- Skill复选框(从Skill列表加载,支持版本选择)
- 系统提示词输入框
- "应用"按钮保存配置到store

### Task 6.5: 开发右侧对话窗口
**Files:** `src/pages/Agent/ChatWindow.tsx`, `src/pages/Agent/MessageItem.tsx`
**内容:**
- 顶部会话信息 + "新建会话"按钮
- 中间消息列表(滚动区域)
- 底部输入框 + "发送"按钮

### Task 6.6: 实现SSE流式对话
**Files:** `src/pages/Agent/index.tsx`
**内容:**
1. 点击发送时组装请求参数
2. 创建SSE客户端订阅
3. 处理各种事件:
   - message → 累积内容更新最后一条消息(打字机效果)
   - message_end → 消息结束标记
   - tool_call_start → 添加工具调用状态卡片
   - tool_call_complete → 更新工具调用结果
   - tool_call_fail → 显示失败状态
   - error → 显示错误提示
   - done → 对话结束
   - title → 更新会话标题(如果需要显示)

### Task 6.7: 开发工具调用状态组件
**Files:** `src/pages/Agent/ToolCallCard.tsx`
**内容:** Card组件展示工具名称、状态(calling/completed/failed)、结果

### Task 6.8: 实现会话管理
**内容:** 
- 新建会话:清空messages,生成新conversationId
- 重置对话:清空store

### Task 6.9: 测试Agent模块
**验收:**
- 配置面板正常工作
- SSE流式返回正常
- 打字机效果流畅
- 工具调用状态正确展示
- 错误处理正常

### Task 6.10: 提交阶段6代码
```bash
git commit -m "feat(agent): implement chat ui with sse streaming and tool call status"
```

---

## 阶段7: 联调优化与Git Submodule配置

**目标:** 完成全流程联调、优化、文档编写

### Task 7.1: 前后端联调测试
**内容:**
1. 启动后端服务
2. 逐个模块测试所有功能
3. 记录并修复发现的问题

### Task 7.2: 异常场景容错
**内容:**
- 网络超时处理
- 后端服务不可用提示
- 数据为空的空状态展示
- 表单校验完善

### Task 7.3: UI细节优化
**内容:**
- 添加Loading状态(Spin组件)
- 添加Empty状态(Empty组件)
- 优化错误提示(message/notification)
- 调整样式细节

### Task 7.4: 响应式布局适配
**内容:** 使用tailwindcss媒体查询,适配小屏幕(1024px以下)

### Task 7.5: 配置.gitignore
**Files:** `mooc-manus-web/.gitignore`
**内容:**
```
node_modules
dist
.env.local
.DS_Store
```

### Task 7.6: 编写项目README
**Files:** `mooc-manus-web/README.md`
**内容:**
- 项目介绍
- 技术栈
- 环境要求
- 安装启动说明
- 目录结构说明
- 开发规范

### Task 7.7: 执行Git Submodule配置
**步骤:**
1. 在GitHub创建 mooc-manus-web 远程仓库
2. 推送前端代码到远程仓库
3. 在父目录执行 init-repo.sh 脚本
4. 验证Submodule配置正确

### Task 7.8: 编写使用文档
**Files:** `mooc-manus-web/docs/USER_GUIDE.md`
**内容:**
- 各模块使用说明
- SSE流式对话注意事项
- 常见问题FAQ

### Task 7.9: 最终验收
**清单:**
- [ ] 所有功能正常运行
- [ ] 无控制台错误和警告
- [ ] 代码符合ESLint规范
- [ ] UI交互流畅
- [ ] Git Submodule配置正确
- [ ] 文档完整

### Task 7.10: 项目交付
```bash
git add .
git commit -m "chore: final polish and documentation"
git push
```

---

**阶段7完成标志:**
- ✅ 所有功能经过联调测试
- ✅ UI细节优化完成
- ✅ Git Submodule配置完成
- ✅ 文档完整
- ✅ 项目可交付

---

## 实施建议

### 开发顺序
1. **严格按阶段1→7顺序**,不要跳跃
2. **每个Task完成后立即提交**,保持Git历史清晰
3. **阶段2完成后立即联调测试**,确认后端连接正常

### TDD实践
- **工具函数**(utils/format.ts等)先写测试
- **API封装**先写测试
- **React组件**以手动测试为主(UI测试成本高)
- **状态管理**可写单元测试(可选)

### 常见问题
1. **@别名不生效**: 检查vite.config.ts和tsconfig.json配置
2. **SSE连接失败**: 检查后端服务是否启动,代理配置是否正确
3. **Ant Design样式异常**: 确认已在main.tsx或index.css中引入样式
4. **Git Submodule推送失败**: 先推送子仓库,再推送总仓库

### 性能优化时机
- 基础功能完成后(阶段7)再考虑性能优化
- 使用React DevTools Profiler分析性能瓶颈
- 按需优化,不要过早优化

---

**完整实施计划结束**

**预计工作量:** 2-3周(1人全职)
**关键里程碑:**
- Week 1: 完成阶段1-3(基础设施 + 模型配置管理)
- Week 2: 完成阶段4-5(工具管理 + Skill管理)
- Week 3: 完成阶段6-7(智能体对话 + 联调优化)

