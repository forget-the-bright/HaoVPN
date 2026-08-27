// Package maintenance 提供服务端后台维护任务（数据保留清理等），与 HTTP api 解耦。
//
// 上游：serverapp 在启动后调用 StartRetentionLoop。
// 下游：persist、logstore、config、safeutil。
// 并发：StartRetentionLoop 在独立 goroutine 运行 ticker；RunDataRetention 可被并发调用但通常单线程。
// 不变量：cfg 为 nil 或 store 为 nil 时 RunDataRetention 直接返回，不 panic。
package maintenance

import (
	"context"
	"time"

	"haovpn/internal/config"
	"haovpn/internal/logstore"
	"haovpn/internal/logger"
	"haovpn/internal/persist"
	"haovpn/internal/safeutil"
)

// RunDataRetention 清理超期审计、连接事件与结构化历史日志。
//
// 参数：store — haovpn.db；logs — 可选 logs.db；cfg — 保留天数配置。
// 副作用：DELETE 超期行；成功时打 Info 日志。
func RunDataRetention(store *persist.Store, logs *logstore.Store, cfg *config.ServerConfig) {
	if store == nil || cfg == nil {
		return
	}
	now := time.Now()

	if days := cfg.Database.AuditRetentionDays; days > 0 {
		cutoff := now.AddDate(0, 0, -days)
		n, err := store.PruneAuditLogs(cutoff)
		if err != nil {
			logger.Warn("审计日志清理失败: %v", err)
		} else if n > 0 {
			logger.Info("已清理审计日志 %d 条（保留 %d 天）", n, days)
		}
	}

	if days := cfg.Database.ConnectionEventsRetentionDays; days > 0 {
		cutoff := now.AddDate(0, 0, -days)
		n, err := store.PruneConnectionEvents(cutoff)
		if err != nil {
			logger.Warn("连接事件清理失败: %v", err)
		} else if n > 0 {
			logger.Info("已清理连接事件 %d 条（保留 %d 天）", n, days)
		}
	}

	if logs != nil && cfg.HistoryLogEnabled() {
		if days := cfg.Log.HistoryRetentionDays; days > 0 {
			cutoff := now.AddDate(0, 0, -days)
			n, err := logs.Prune(cutoff)
			if err != nil {
				logger.Warn("历史日志库清理失败: %v", err)
			} else if n > 0 {
				logger.Info("已清理历史日志 %d 条（保留 %d 天）", n, days)
			}
		}
	}
}

// StartRetentionLoop 启动时执行一次并每日重复数据保留清理。
//
// 参数：ctx — 取消时停止 ticker；其余同 RunDataRetention。
// 副作用：启动 goroutine；阻塞直到 ctx 取消（在 goroutine 内 RunTicker）。
func StartRetentionLoop(ctx context.Context, store *persist.Store, logs *logstore.Store, cfg *config.ServerConfig) {
	RunDataRetention(store, logs, cfg)
	go func() {
		safeutil.RunTicker(ctx, 24*time.Hour, func(context.Context) {
			RunDataRetention(store, logs, cfg)
		})
	}()
}
