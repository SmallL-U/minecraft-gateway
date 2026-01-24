package logx

import "go.uber.org/zap"

var logger = func() *zap.SugaredLogger {
	production, _ := zap.NewProduction()
	return production.Sugar()
}()

func GetLogger() *zap.SugaredLogger {
	return logger
}
