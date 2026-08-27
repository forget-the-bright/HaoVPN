// Package health 提供服务端启动自检与 /api/v1/health 就绪探针逻辑。
//
// Checker 在 serverapp.Run 启动早期执行；结果影响管理台展示与运维判断。
package health
