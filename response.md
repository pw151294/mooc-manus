## 主要依赖（来自pyproject.toml）：

1. **核心Web框架**：
    - `fastapi>=0.124.0` - 现代、快速的Web框架
    - `uvicorn[standard]>=0.38.0` - ASGI服务器

2. **数据验证和配置**：
    - `pydantic>=2.12.5` - 数据验证和设置管理
    - `pydantic-settings>=2.12.0` - Pydantic配置管理

3. **文件处理**：
    - `python-multipart>=0.0.20` - 处理multipart/form-data请求

4. **MCP协议**：
    - `mcp>=1.26.0` - Model Context Protocol支持

5. **包管理**：
    - `pip>=25.3` - Python包管理器

## 完整依赖列表（来自requirements.txt）：

除了上述主要依赖外，requirements.txt还包含了以下间接依赖：

- `annotated-doc==0.0.4`
- `annotated-types==0.7.0`
- `anyio==4.12.0`
- `click==8.3.1`
- `colorama==0.4.6` (Windows平台)
- `exceptiongroup==1.3.1` (Python < 3.11)
- `h11==0.16.0`
- `httptools==0.7.1`
- `idna==3.11`
- `pydantic-core==2.41.5`
- `python-dotenv==1.2.1`
- `pyyaml==6.0.3`
- `starlette==0.50.0`
- `typing-extensions==4.15.0`
- `typing-inspection==0.4.2`
- `uvloop==0.22.1` (非Windows/PyPy平台)
- `watchfiles==1.1.1`
- `websockets==15.0.1`

## 项目特点：

1. **FastAPI项目**：这是一个基于FastAPI的Web API项目
2. **MCP支持**：集成了Model Context Protocol
3. **Python版本**：要求Python >= 3.10
4. **功能模块**：
    - 文件操作模块
    - Shell命令执行模块
    - Supervisor进程管理模块

## 安装建议：

您可以使用以下任一方式安装依赖：

1. **使用uv**（推荐）：
   ```bash
   uv pip install -e .
   ```

2. **使用pip**：
   ```bash
   pip install -r requirements.txt
   ```

3. **使用poetry**（如果配置了）：
   ```bash
   poetry install
   ```

项目看起来是一个沙箱管理系统，提供了文件操作、Shell命令执行和进程管理等功能。