# zap-logman
The log file manager for zap log

The project is aimed at managing the log files by time, eg: daily, hourly. The inspiration comes from https://github.com/natefinch/lumberjack.

## usage
```go
w := &logman.Manager{
  Filename:  "./log/foo.log",
  MaxRawAge: 10,
  MaxAge:    5, // days
  Compress:  true,
  Timing:    logman.HOURLY,
  Level:     zapcore.DebugLevel,
}
cfg := zap.NewProductionEncoderConfig()
cfg.TimeKey = "time"
cfg.EncodeTime = zapcore.TimeEncoderOfLayout("2006-01-02 15:04:05.000000000")
core := logman.NewCore(
  zapcore.NewJSONEncoder(cfg),
  w,
)
```
