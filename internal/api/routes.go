package api

// API路径常量定义 - 供客户端和服务端共享使用
const (
	// API版本前缀
	APIVersionPrefix = "/api/v1"

	// 健康检查
	HealthPath = "/health"

	// TSS操作路径
	KeygenPath  = "/keygen"
	SignPath    = "/sign"
	ResharePath = "/reshare"

	// 操作查询路径
	OperationsPath = "/operations"

	// 完整的API路径
	FullKeygenPath     = APIVersionPrefix + KeygenPath
	FullSignPath       = APIVersionPrefix + SignPath
	FullResharePath    = APIVersionPrefix + ResharePath
	FullOperationsPath = APIVersionPrefix + OperationsPath
)

// GetOperationPath 返回特定操作的完整路径
func GetOperationPath(operationID string) string {
	return FullOperationsPath + "/" + operationID
}

// API路径模式（用于路由注册）
const (
	OperationPathPattern = OperationsPath + "/:operation_id"
	KeyMetadataPath      = "/keys/:key_id"
)
