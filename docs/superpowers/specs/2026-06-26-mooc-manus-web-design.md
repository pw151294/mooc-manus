# mooc-manus-web 前端项目技术设计方案

> **项目定位:** 可视化智能体编排管理平台  
> **设计日期:** 2026-06-26  
> **后端仓库:** https://github.com/pw151294/mooc-manus.git

---

## 一、项目概述

### 1.1 项目背景

基于已有 `mooc-manus` 智能体编排平台后端工程（Go语言 + Gin + GORM + PostgreSQL），新建前端项目 `mooc-manus-web`，实现模型、工具、Skill、智能体对话编排全生命周期可视化配置与调试能力。

### 1.2 核心功能模块

| 模块 | 功能说明 | 后端数据源 |
|------|---------|-----------|
| 模型配置管理 | 模型配置的增删改查、参数配置 | `app_config` 表 |
| 工具包管理 | 工具供应商和工具函数的维护、启用禁用 | `tool_provider`、`tool_function` 表 |
| Skill管理 | Skill基础信息、多版本管理、Provider绑定、上下线管控 | `skill`、`skill_version`、`skill_provider` 表 |
| 智能体对话 | 可视化能力装配 + SSE流式多轮对话 | Agent Chat 接口 |

### 1.3 技术约束

- 前后端通过 **RESTful API** + **SSE事件流** 交互
- SSE采用后端定制化事件格式（`event: <type>\ndata: <json>\n\n`）
- 不支持流式中断和自动重连，超时时间**1分钟**
- 前后端通过 **Git Submodule** 统一管理在 `mooc-manus-all` 总仓库

---

## 二、技术选型

### 2.1 核心框架与工具链

| 技术栈 | 版本 | 选型理由 |
|--------|------|---------|
| **React** | 18.x | 类型安全、生态成熟、组件化开发 |
| **TypeScript** | 5.x | 类型安全、智能提示、减少运行时错误 |
| **Vite** | 5.x | 极速构建、HMR热更新、开箱即用TypeScript支持 |
| **React Router** | v6 | 声明式路由管理 |
| **Zustand** | latest | 轻量级状态管理，比Redux简单，比Context性能好 |

### 2.2 UI组件库

| 库 | 用途 |
|----|------|
| **Ant Design** | 5.x企业级UI组件库，卡片、表单、弹窗开箱即用 |
| **@ant-design/icons** | 图标库 |
| **tailwindcss** | 原子化CSS，快速调整样式细节 |

### 2.3 网络请求

| 库 | 用途 |
|----|------|
| **axios** | HTTP客户端，统一请求封装 |
| **EventSource** (原生API) | SSE事件流订阅 |

### 2.4 工具库

- **dayjs** - 时间格式化
- **lodash-es** - 工具函数集合
- **uuid** - 生成唯一ID

### 2.5 代码规范工具

- **ESLint** + **Prettier** - 代码风格统一
- **husky** + **lint-staged** - Git提交前自动检查

---

## 三、目录架构设计

```
mooc-manus-web/
├── public/                    # 静态资源
├── src/
│   ├── api/                   # 接口层
│   │   ├── request.ts         # Axios实例配置
│   │   ├── sse.ts             # SSE工具类封装
│   │   ├── modules/           # 按业务模块拆分接口
│   │   │   ├── appConfig.ts   # 模型配置接口
│   │   │   ├── tool.ts        # 工具管理接口
│   │   │   ├── skill.ts       # Skill接口
│   │   │   └── agent.ts       # 智能体对话接口
│   │   └── types/             # 接口类型定义
│   ├── components/            # 公共组件
│   │   ├── Layout/            # 布局组件
│   │   ├── SSEChat/           # SSE流式对话组件
│   │   └── ...
│   ├── pages/                 # 页面组件
│   │   ├── AppConfig/         # 模型配置管理
│   │   ├── Tool/              # 工具管理
│   │   ├── Skill/             # Skill管理
│   │   └── Agent/             # 智能体对话
│   ├── store/                 # 状态管理
│   │   ├── appConfig.ts
│   │   ├── tool.ts
│   │   └── agent.ts
│   ├── types/                 # 全局类型定义
│   │   ├── appConfig.ts
│   │   ├── tool.ts
│   │   ├── skill.ts
│   │   ├── agent.ts
│   │   └── sse.ts             # SSE事件类型
│   ├── utils/                 # 工具函数
│   │   ├── format.ts          # 格式化工具
│   │   └── validator.ts       # 校验工具
│   ├── constants/             # 常量定义
│   ├── hooks/                 # 自定义Hooks
│   ├── router/                # 路由配置
│   ├── App.tsx
│   ├── main.tsx
│   └── vite-env.d.ts
├── .env.development           # 开发环境配置
├── .env.production            # 生产环境配置
├── vite.config.ts
├── tsconfig.json
├── package.json
└── README.md
```

**架构特点:**
- **按职能分层** - api/components/pages/store/types 清晰分离
- **模块化接口** - api/modules 按业务模块拆分，易维护
- **类型集中管理** - types目录统一管理类型定义，避免循环依赖
- **组件复用** - components存放公共组件，pages存放页面级组件
---

## 四、路由规划

### 4.1 路由结构

```
/                          # 重定向到 /agent
├── /agent                 # 智能体对话(左右分栏:配置面板+聊天窗口)
├── /model-config          # 模型配置管理(卡片式列表)
├── /tools                 # 工具管理
│   ├── /providers         # 工具供应商管理(卡片式)
│   └── /functions         # 工具函数管理(卡片式)
└── /skills                # Skill管理(混合式)
    ├── /                  # 主页面(左侧Provider树,右侧Skill卡片,详情用Modal)
    └── /import-tasks      # 导入任务管理(独立页面,SSE订阅进度)
```

### 4.2 路由设计原则

- **扁平化结构** - 避免深层嵌套,降低路由复杂度
- **语义化路径** - URL路径清晰表达页面功能
- **模块化拆分** - 工具模块分Provider和Function两个子路由
- **混合式布局** - Skill主页面采用混合式,导入任务独立

---

## 五、界面设计方案

### 5.1 智能体对话模块 - 左右分栏式

**设计原则:** 配置和对话同屏可见,适合频繁调整配置的场景

**布局方案:**
- 左侧配置面板固定宽度 300px
- 右侧对话窗口自适应宽度
- 配置面板包含:模型选择、工具包多选、Skill多选、系统提示词输入
- 对话窗口包含:会话管理、消息列表、输入框

**交互流程:**
1. 用户在左侧选择模型配置(下拉框,来自AppConfig列表)
2. 勾选需要的工具包(复选框,来自Tool Function列表)
3. 勾选需要的Skills(复选框,支持版本选择,来自Skill列表)
4. (可选)填写系统提示词
5. 点击"应用"保存配置到Zustand store
6. 在右侧输入消息,点击发送
7. 组装请求参数: `{ appConfigId, functionIds, skillRefs, systemPrompt, query }`
8. 调用SSE接口,实时渲染流式响应

**SSE事件处理:**
- `message` 事件 → 累积内容,实现打字机效果
- `message_end` 事件 → 消息结束标记
- `tool_call_start` → 显示工具调用状态卡片
- `tool_call_complete` → 更新工具调用结果
- `tool_call_fail` → 显示工具调用失败提示
- `error` → 显示错误提示
- `done` → 对话结束,可继续提问

### 5.2 管理模块 - 卡片式风格

**适用页面:**
- 模型配置管理 (AppConfig)
- 工具供应商管理 (Tool Provider)
- 工具包管理 (Tool Function)

**设计特点:**
- 视觉层次清晰,现代感强
- 易于展示状态和标签
- 移动端友好,响应式布局简单
- Grid布局自适应,单卡片宽度 300px 左右

**卡片内容结构:**
```
┌─────────────────────┐
│ 标题      [状态标签]│
│ 副标题/描述          │
├─────────────────────┤
│ 关键参数展示         │
│ (如: 温度/Token等)   │
├─────────────────────┤
│ [编辑] [删除]       │
└─────────────────────┘
```

**交互操作:**
- 新增: 页面右上角"+ 新增"按钮,弹出Modal表单
- 编辑: 卡片内"编辑"按钮,弹出Modal表单回填数据
- 删除: 卡片内"删除"按钮,Popconfirm二次确认
- 搜索: 页面顶部搜索框,支持关键词过滤

### 5.3 Skill管理 - 混合式(列表+详情)

**设计原则:** Skill和Provider在同一页面,导入任务独立

**布局方案:**
```
┌─ Skill管理 ───────────────────────────┐
│ [Skills] [导入任务] Tab               │
├──────┬────────────────────────────────┤
│Provider树 (左侧 250px)  │ Skill卡片列表 (右侧) │
│                        │                      │
│ 📁 官方                │ [搜索] [新增Skill]  │
│   ├ A2A Agents        │                      │
│   └ SRE Tools         │ ┌────┐ ┌────┐       │
│                        │ │Skill│ │Skill│      │
│ 📁 自定义              │ │ v1 │ │ v2 │       │
│   └ My Skills         │ └────┘ └────┘       │
│                        │                      │
│ [+ 导入Provider]       │                      │
└────────────────────────┴──────────────────────┘
```

**交互流程:**
1. 左侧Provider树支持折叠/展开
2. 点击Provider节点,右侧过滤对应Skill卡片
3. 点击Skill卡片,弹出Modal展示详情:
   - 基本信息(名称、描述、状态、图标)
   - 版本列表(支持版本切换、回滚、导出)
   - 文件列表(支持下载)
4. 点击"导入任务"Tab切换到导入任务管理页面
5. 导入任务页面使用SSE订阅进度,实时更新进度条和日志


---

## 六、SSE流式对话技术方案

### 6.1 后端SSE事件格式

后端使用 `internal/infra/external/sse/sse.go` 的 `SendEvent` 函数发送事件:

```go
// 事件格式
event: <eventName>
data: <jsonData>

```

### 6.2 SSE事件类型定义

基于 `internal/domains/models/events/constants.go` 定义的事件类型:

```typescript
// src/types/sse.ts
type SSEEventType = 
  | 'message'              // 流式消息片段
  | 'message_end'          // 消息结束
  | 'tool_call_start'      // 工具调用开始
  | 'tool_call_complete'   // 工具调用完成
  | 'tool_call_fail'       // 工具调用失败
  | 'error'                // 错误
  | 'done'                 // 对话结束
  | 'title'                // 标题生成
  | 'plan_create_success'  // 计划创建成功
  | 'step_start'           // 步骤开始
  | 'step_complete';       // 步骤完成

// 对应后端 MessageEvent
interface MessageEventData {
  id: string;
  conversationId: string;
  type: 'message' | 'message_end';
  timestamp: string;
  role: 'user' | 'assistant';
  message: string;
  attachments?: any[];
}

// 对应后端 ToolEvent
interface ToolEventData {
  id: string;
  conversationId: string;
  type: 'tool_call_start' | 'tool_call_complete' | 'tool_call_fail';
  timestamp: string;
  tool_call_id: string;
  tool_name: string;
  function_name: string;
  function_args: string;
  function_result?: any;
  status: 'calling' | 'completed' | 'failed';
}

// 对应后端 ErrorEvent
interface ErrorEventData {
  id: string;
  conversationId: string;
  type: 'error';
  timestamp: string;
  error: string;
}

// 对应后端 DoneEvent
interface DoneEventData {
  id: string;
  conversationId: string;
  type: 'done';
  timestamp: string;
}
```

### 6.3 SSE客户端封装

```typescript
// src/api/sse.ts
class SSEClient {
  private eventSource: EventSource | null = null;
  private timeout: number = 60000; // 1分钟超时
  private timeoutTimer: NodeJS.Timeout | null = null;
  private isActive: boolean = false; // 防止重复订阅

  subscribe(url: string, handlers: {
    onOpen?: () => void;
    onEvent: (eventType: string, data: any) => void;
    onError?: (error: any) => void;
    onComplete?: () => void;
  }) {
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
      'message', 'message_end', 'tool_call_start', 
      'tool_call_complete', 'tool_call_fail', 
      'error', 'done', 'title'
    ];

    eventTypes.forEach(type => {
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

  private resetTimeout() {
    if (this.timeoutTimer) clearTimeout(this.timeoutTimer);
    this.timeoutTimer = setTimeout(() => {
      this.close();
      console.warn('SSE connection timeout');
    }, this.timeout);
  }

  close() {
    this.isActive = false;
    if (this.timeoutTimer) clearTimeout(this.timeoutTimer);
    if (this.eventSource) {
      this.eventSource.close();
      this.eventSource = null;
    }
  }
}

export default SSEClient;
```

### 6.4 使用示例

```typescript
// 在对话组件中使用
const handleSendMessage = async (message: string) => {
  const sseClient = new SSEClient();
  const url = `/api/agent/chat?query=${encodeURIComponent(message)}&...`;
  
  let currentMessageContent = '';

  sseClient.subscribe(url, {
    onEvent: (type, data) => {
      switch(type) {
        case 'message':
          currentMessageContent += data.message;
          updateLastMessage(currentMessageContent);
          break;
        case 'tool_call_start':
          addToolCallStatus({ id: data.tool_call_id, name: data.tool_name, status: 'calling' });
          break;
        case 'tool_call_complete':
          updateToolCallStatus(data.tool_call_id, 'completed', data.function_result);
          break;
        case 'tool_call_fail':
          updateToolCallStatus(data.tool_call_id, 'failed');
          break;
        case 'error':
          message.error(data.error);
          break;
      }
    },
    onError: () => message.error('连接中断'),
    onComplete: () => console.log('对话完成')
  });
};
```

### 6.5 技术要点

1. **不支持中断和重连** - 连接失败直接提示,符合需求
2. **超时1分钟** - 通过定时器实现,收到消息自动重置
3. **打字机效果** - 累积message事件内容,实时更新UI
4. **事件分类处理** - 根据事件类型展示不同UI状态


---

## 七、全局请求封装方案

### 7.1 Axios实例配置

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
  (error) => Promise.reject(error)
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

### 7.2 接口模块示例

```typescript
// src/api/modules/appConfig.ts
import request from '../request';
import type { 
  AppConfigDTO, 
  AppConfigCreateRequest, 
  AppConfigUpdateRequest 
} from '@/types/appConfig';

// 创建AppConfig
export const createAppConfig = (data: AppConfigCreateRequest) => {
  return request.post<{ id: string }>('/api/app/config', data);
};

// 更新AppConfig
export const updateAppConfig = (id: string, data: AppConfigUpdateRequest) => {
  return request.put(`/api/app/config/${id}`, { ...data, appConfigId: id });
};

// 获取单个AppConfig
export const getAppConfig = (id: string) => {
  return request.get<AppConfigDTO>(`/api/app/config/${id}`);
};

// 获取AppConfig列表
export const listAppConfigs = () => {
  return request.get<AppConfigDTO[]>('/api/app/config');
};

// 删除AppConfig
export const deleteAppConfig = (id: string) => {
  return request.delete(`/api/app/config/${id}`);
};
```

### 7.3 环境变量配置

**.env.development:**
```bash
VITE_API_BASE_URL=http://localhost:8080
```

**.env.production:**
```bash
VITE_API_BASE_URL=https://your-production-api.com
```

### 7.4 Vite跨域代理配置

**vite.config.ts:**

```typescript
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
  server: {
    port: 3000,
    proxy: {
      '/api': {
        target: 'http://localhost:8080',
        changeOrigin: true,
        // 如需去除/api前缀,取消下行注释:
        // rewrite: (path) => path.replace(/^\/api/, '')
      }
    }
  },
  resolve: {
    alias: {
      '@': '/src'
    }
  }
})
```

**说明:**
- `target`: 后端服务地址
- `changeOrigin`: true表示修改请求头的origin字段
- `rewrite`: 可选,是否重写路径(去除/api前缀)

### 7.5 技术特点

- **适配后端响应格式** - 后端无统一封装,直接透传data
- **统一错误处理** - 根据HTTP状态码映射错误提示
- **环境变量管理** - 开发/生产环境API地址分离
- **TypeScript类型安全** - 所有接口都有完整类型定义
- **跨域代理** - 开发环境通过Vite代理解决跨域问题


---

## 八、状态管理方案

### 8.1 Zustand状态管理

使用 Zustand 进行轻量级状态管理,按业务模块拆分store。

### 8.2 智能体对话状态示例

```typescript
// src/store/agent.ts
import { create } from 'zustand';
import type { AppConfigDTO, ToolFunctionDTO, SkillWithVersion } from '@/types';

interface Message {
  id: string;
  role: 'user' | 'assistant';
  content: string;
  timestamp: number;
  toolCalls?: ToolCallStatus[];
}

interface ToolCallStatus {
  id: string;
  toolName: string;
  functionName: string;
  status: 'calling' | 'completed' | 'failed';
  result?: any;
}

interface AgentState {
  // 能力装配配置
  selectedConfig: AppConfigDTO | null;
  selectedTools: ToolFunctionDTO[];
  selectedSkills: SkillWithVersion[];
  systemPrompt: string;

  // 对话状态
  messages: Message[];
  conversationId: string | null;
  isStreaming: boolean;

  // Actions
  setSelectedConfig: (config: AppConfigDTO | null) => void;
  setSelectedTools: (tools: ToolFunctionDTO[]) => void;
  setSelectedSkills: (skills: SkillWithVersion[]) => void;
  setSystemPrompt: (prompt: string) => void;
  
  addMessage: (message: Message) => void;
  updateLastMessage: (content: string) => void;
  addToolCallStatus: (toolCall: ToolCallStatus) => void;
  
  startStreaming: () => void;
  stopStreaming: () => void;
  resetConversation: () => void;
}

export const useAgentStore = create<AgentState>((set) => ({
  selectedConfig: null,
  selectedTools: [],
  selectedSkills: [],
  systemPrompt: '',
  messages: [],
  conversationId: null,
  isStreaming: false,

  setSelectedConfig: (config) => set({ selectedConfig: config }),
  setSelectedTools: (tools) => set({ selectedTools: tools }),
  setSelectedSkills: (skills) => set({ selectedSkills: skills }),
  setSystemPrompt: (prompt) => set({ systemPrompt: prompt }),

  addMessage: (message) => set((state) => ({
    messages: [...state.messages, message]
  })),

  updateLastMessage: (content) => set((state) => ({
    messages: state.messages.map((msg, idx) =>
      idx === state.messages.length - 1 ? { ...msg, content } : msg
    )
  })),

  addToolCallStatus: (toolCall) => set((state) => {
    const messages = [...state.messages];
    const lastMsg = messages[messages.length - 1];
    if (lastMsg && lastMsg.role === 'assistant') {
      lastMsg.toolCalls = [...(lastMsg.toolCalls || []), toolCall];
    }
    return { messages };
  }),

  startStreaming: () => set({ isStreaming: true }),
  stopStreaming: () => set({ isStreaming: false }),
  
  resetConversation: () => set({
    messages: [],
    conversationId: null,
    isStreaming: false
  }),
}));
```

### 8.3 Zustand优势

- API简洁,无Provider包裹
- TypeScript类型完整
- 支持React DevTools调试
- 按需引入,不影响性能


---

## 九、Git Submodule多仓库管理方案

### 9.1 仓库结构

```
mooc-manus-all/              # 总仓库
├── mooc-manus/              # 后端子仓库 (已存在)
├── mooc-manus-web/          # 前端子仓库 (新建)
├── .gitmodules              # Submodule配置文件
└── README.md                # 仓库使用说明
```

### 9.2 初始化脚本

**`init-repo.sh` - 总仓库初始化脚本:**

```bash
#!/bin/bash
set -e

echo "🚀 初始化 mooc-manus-all 总仓库..."

# 1. 创建总仓库目录
mkdir -p mooc-manus-all
cd mooc-manus-all
git init
echo "✅ 总仓库初始化完成"

# 2. 添加后端子仓库
echo "📦 添加后端子仓库..."
git submodule add https://github.com/pw151294/mooc-manus.git mooc-manus
echo "✅ 后端子仓库添加完成"

# 3. 创建前端子仓库
echo "📦 创建前端子仓库..."
echo "⚠️  请确保已在GitHub创建 mooc-manus-web 远程仓库"
read -p "请输入前端仓库URL (如: https://github.com/pw151294/mooc-manus-web.git): " FRONTEND_REPO_URL

if [ -z "$FRONTEND_REPO_URL" ]; then
  echo "❌ 未提供前端仓库URL,退出"
  exit 1
fi

# 克隆前端空仓库或初始化本地仓库
git clone "$FRONTEND_REPO_URL" mooc-manus-web || {
  # 如果远程仓库不存在,本地初始化后关联
  mkdir mooc-manus-web
  cd mooc-manus-web
  git init
  echo "# mooc-manus-web" > README.md
  git add README.md
  git commit -m "Initial commit"
  git remote add origin "$FRONTEND_REPO_URL"
  git push -u origin master || echo "⚠️  推送失败,请手动创建远程仓库后再推送"
  cd ..
}
echo "✅ 前端子仓库初始化完成"

# 4. 添加前端子仓库为Submodule
cd ..
rm -rf mooc-manus-all/mooc-manus-web  # 删除刚才clone的目录
cd mooc-manus-all
git submodule add "$FRONTEND_REPO_URL" mooc-manus-web
echo "✅ 前端子仓库作为Submodule添加完成"

# 5. 创建总仓库README
cat > README.md << 'READMEEOF'
# mooc-manus-all

智能体编排平台 - 前后端统一管理仓库

## 仓库结构

- `mooc-manus/` - 后端服务 (Go)
- `mooc-manus-web/` - 前端应用 (React + TypeScript)

## 快速开始

### 1. 克隆仓库(含子仓库)

```bash
git clone --recursive <总仓库URL>
```

或者先克隆总仓库,再拉取子仓库:

```bash
git clone <总仓库URL>
cd mooc-manus-all
git submodule init
git submodule update
```

### 2. 启动后端

```bash
cd mooc-manus
# 参考后端README启动说明
```

### 3. 启动前端

```bash
cd mooc-manus-web
npm install
npm run dev
```

## 子仓库管理

### 拉取子仓库最新代码

```bash
# 更新所有子仓库
git submodule update --remote

# 更新指定子仓库
cd mooc-manus
git pull origin master
cd ..
git add mooc-manus
git commit -m "Update backend submodule"
```

### 在子仓库中开发

```bash
# 进入子仓库
cd mooc-manus-web

# 切换到工作分支(重要!)
git checkout master

# 正常的Git工作流
git checkout -b feature/new-page
# ... 开发 ...
git add .
git commit -m "feat: add new page"
git push origin feature/new-page

# 回到总仓库,提交子仓库引用变更
cd ..
git add mooc-manus-web
git commit -m "Update frontend submodule"
git push
```

### 提交推送完整流程

```bash
# 1. 在子仓库中提交
cd mooc-manus-web
git add .
git commit -m "feat: implement feature"
git push

# 2. 回到总仓库,更新子仓库引用
cd ..
git add mooc-manus-web
git commit -m "chore: update frontend to latest"
git push
```

## 注意事项

⚠️ **子仓库处于"游离HEAD"状态**

Submodule默认指向特定commit,不在任何分支上。开发前务必:

```bash
cd mooc-manus-web
git checkout master  # 或其他开发分支
```

⚠️ **不要忘记推送子仓库**

修改子仓库后,需要先推送子仓库,再推送总仓库:

```bash
# 错误示例 ❌
cd mooc-manus-all
git add mooc-manus-web
git commit -m "update"
git push  # 子仓库未推送,其他人无法获取最新代码!

# 正确示例 ✅
cd mooc-manus-web
git push  # 先推送子仓库
cd ..
git add mooc-manus-web
git commit -m "update"
git push  # 再推送总仓库
```
READMEEOF

# 6. 提交总仓库
git add .
git commit -m "Initial commit: setup submodules structure"

echo "🎉 mooc-manus-all 总仓库初始化完成!"
echo ""
echo "📁 目录结构:"
echo "  mooc-manus-all/"
echo "  ├── mooc-manus/      (后端子仓库)"
echo "  ├── mooc-manus-web/  (前端子仓库)"
echo "  └── README.md"
```

### 9.3 日常开发流程

**开发者 - 修改前端:**

```bash
# 1. 克隆总仓库
git clone --recursive <总仓库URL>
cd mooc-manus-all/mooc-manus-web

# 2. 切换到工作分支
git checkout master

# 3. 开发
git checkout -b feat/new-feature
# ... coding ...
git commit -m "feat: add new feature"

# 4. 推送子仓库
git push origin feat/new-feature

# 5. 更新总仓库引用
cd ..
git add mooc-manus-web
git commit -m "chore: update frontend submodule"
git push
```

**开发者 - 拉取最新代码:**

```bash
cd mooc-manus-all
git pull
git submodule update --remote --merge
```


---

## 十、实施阶段划分

### 10.1 阶段1: 基础工程初始化

**目标:** 搭建可运行的前端项目骨架

**任务清单:**
1. 使用Vite创建React + TypeScript项目
2. 配置Ant Design、tailwindcss、路由
3. 配置ESLint、Prettier、husky
4. 配置环境变量(.env.development)
5. 创建基础目录结构
6. 配置代理解决跨域问题(vite.config.ts)
7. 创建基础Layout组件

**验收标准:**
- 项目可成功启动(`npm run dev`)
- 无编译错误和警告
- 访问localhost可看到初始页面

---

### 10.2 阶段2: 全局公共能力封装

**目标:** 完成核心基础设施封装

**任务清单:**
1. 封装Axios请求拦截器和响应拦截器
2. 封装SSE客户端类(SSEClient)
3. 定义全局类型(sse.ts, appConfig.ts等)
4. 创建路由配置(router/index.tsx)
5. 创建全局Layout布局组件
6. 创建通用组件(Loading、Empty等)

**验收标准:**
- 可成功调用后端健康检查接口(`/api/status`)
- SSE客户端可正常订阅和接收事件
- 路由跳转正常

---

### 10.3 阶段3: 模型配置管理模块

**目标:** 完成AppConfig模块的完整CRUD

**任务清单:**
1. 定义AppConfig类型(`types/appConfig.ts`)
2. 实现AppConfig接口(`api/modules/appConfig.ts`)
3. 创建AppConfig状态管理(`store/appConfig.ts`)
4. 开发AppConfig卡片列表页面
5. 开发新增/编辑Modal表单
6. 实现搜索、删除功能

**验收标准:**
- 可查看AppConfig列表
- 可新增、编辑、删除AppConfig
- 表单校验正常
- 操作成功后列表自动刷新

---

### 10.4 阶段4: 工具管理模块

**目标:** 完成工具供应商和工具函数管理

**任务清单:**
1. 定义Tool相关类型
2. 实现Tool相关接口
3. 开发工具供应商管理页面(卡片式)
4. 开发工具函数管理页面(卡片式)
5. 实现Provider和Function的关联查询

**验收标准:**
- 可管理工具供应商
- 可管理工具函数
- 可按Provider筛选Function
- 所有CRUD操作正常

---

### 10.5 阶段5: Skill管理模块

**目标:** 完成Skill的混合式管理界面

**任务清单:**
1. 定义Skill相关类型(Skill、Version、Provider)
2. 实现Skill相关接口
3. 开发Skill主页面(左侧Provider树 + 右侧Skill卡片)
4. 开发Skill详情Modal(基本信息、版本列表、文件列表)
5. 开发导入任务页面(SSE订阅进度)
6. 实现版本管理功能(切换、回滚、导出)

**验收标准:**
- Provider树可正常展开折叠
- 点击Provider可过滤Skill
- Skill详情Modal可查看完整信息
- 导入任务可实时显示进度
- 版本管理功能正常

---

### 10.6 阶段6: 智能体对话模块

**目标:** 完成能力装配 + SSE流式对话

**任务清单:**
1. 开发左侧配置面板组件
2. 开发右侧对话窗口组件
3. 实现配置项的数据联动(从AppConfig/Tool/Skill获取)
4. 实现SSE流式对话集成
5. 实现打字机效果
6. 实现工具调用状态展示
7. 实现会话管理(新建、重置)

**验收标准:**
- 配置面板可正常选择模型、工具、Skill
- 可发送消息并收到流式响应
- 打字机效果流畅
- 工具调用状态正确展示
- 错误处理正常

---

### 10.7 阶段7: 联调优化与Git Submodule配置

**目标:** 完成全流程联调和仓库管理配置

**任务清单:**
1. 前后端接口联调测试
2. 异常场景容错处理
3. UI细节优化(加载状态、空状态、错误提示)
4. 响应式布局适配
5. 执行init-repo.sh初始化总仓库
6. 配置.gitignore
7. 编写项目README和使用文档

**验收标准:**
- 所有功能正常运行
- 无控制台错误和警告
- UI交互流畅
- Git Submodule配置正确
- 文档完整

---

## 十一、技术风险与对策

| 风险项 | 影响 | 对策 |
|--------|------|------|
| SSE连接不稳定 | 流式对话中断 | 实现1分钟超时机制,超时后友好提示 |
| 后端事件格式变更 | 前端解析失败 | TypeScript类型定义严格对齐后端,接口文档同步 |
| Skill文件上传大小限制 | 导入失败 | 前端校验文件大小,超过限制提前提示 |
| 浏览器兼容性 | 部分浏览器不支持EventSource | 使用polyfill兼容IE11+(如需要) |
| Git Submodule操作复杂 | 开发者易出错 | 提供详细文档和脚本,规范操作流程 |

---

## 十二、性能优化策略

1. **代码分割** - 使用React.lazy和Suspense实现路由级代码分割
2. **列表虚拟化** - 大列表使用react-window优化渲染性能
3. **防抖节流** - 搜索、滚动等高频操作使用lodash防抖节流
4. **图片懒加载** - Skill图标等图片使用懒加载
5. **接口缓存** - AppConfig等配置数据使用store缓存,减少重复请求
6. **SSE连接复用** - 同一会话复用SSE连接,避免频繁创建

---

## 十三、交付清单

### 13.1 代码交付

- ✅ 可运行的前端工程(`mooc-manus-web/`)
- ✅ Git Submodule总仓库(`mooc-manus-all/`)
- ✅ 初始化脚本(`init-repo.sh`)

### 13.2 文档交付

- ✅ 项目README(环境配置、启动说明)
- ✅ Git Submodule使用说明
- ✅ 各模块使用操作说明
- ✅ SSE流式对话使用注意事项
- ✅ 接口对接说明

### 13.3 质量标准

- ✅ 无运行报错
- ✅ 无控制台警告
- ✅ 代码符合ESLint规范
- ✅ 所有功能可正常使用
- ✅ 响应式布局适配

---

**设计方案完成日期:** 2026-06-26  
**预计实施周期:** 2-3周  
**后续迭代方向:** 移动端适配、国际化、主题切换、权限管理

