# 图像生成

本上下文定义 `img-gen` 中用于描述图像生成后端及其选择方式的领域语言。

## Providers

**Provider**：
由 `img-gen` 以一个标准集成形式支持的外部图像生成服务商，例如 OpenAI 或 Google。Provider 由用户所选择的服务标识，而不是由 HTTP 协议、认证方式、端点地址或某个具体模型标识。
_避免使用：protocol、transport、API endpoint、adapter_

**Provider Configuration**：
绑定到某个已选 Provider 的连接设置，例如 API Base URL、API Key 和 Model。Provider Configuration 不会建立或推断 Provider 身份。
_避免使用：Provider、custom endpoint_

**Provider Selection**：
确定一个任务使用哪个 Provider 的过程。它只能来自任务中的明确选择或已配置的默认值，绝不通过检查凭据、端点地址、模型标识或任务能力自动推断。
_避免使用：provider detection、provider inference、auto-selection_

**Model**：
通过某个 Provider、以该 Provider 的模型标识可调用的具体图像能力。Model 的完整身份由 Provider 和模型标识共同构成；单独的模型标识永远不被假定为代表某个 Provider。
_避免使用：Provider、model family、global model ID_
