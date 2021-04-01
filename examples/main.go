package main

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/panjf2000/ants/v2"
	"github.com/pkg/profile"
	"github.com/wk30/zaplogman"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func main() {
	var logger *zap.Logger
	startat := time.Now()
	defer func() {
		log.Printf("duration: %vs", time.Now().Sub(startat).Seconds())
	}()
	defer profile.Start().Stop()
	man := &zaplogman.Man{
		Filename:   "./log/foo.log",
		MaxBackups: 10,
		MaxAge:     5, // days
		Compress:   true,
		Timing:     zaplogman.SECONDLY,
		Level:      zapcore.DebugLevel,
	}
	cfg := zap.NewProductionEncoderConfig()
	cfg.TimeKey = "time"
	cfg.EncodeTime = zapcore.TimeEncoderOfLayout("2006-01-02 15:04:05.000000000")
	core := zaplogman.NewCore(
		zapcore.NewJSONEncoder(cfg),
		man,
	)
	// cfg := zap.NewProductionEncoderConfig()
	// enc := zapcore.NewJSONEncoder(cfg)
	// w, _ := os.OpenFile("./log/foo.log", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	// core := zapcore.NewCore(enc, zapcore.AddSync(w), zapcore.DebugLevel)
	logger = zap.New(core)
	defer logger.Sync()

	defer ants.Release()

	runTimes := 2000000

	demo1 := func() {
		logger.Info("A")
		logger.Debug("B")
		logger.Warn("C")
		logger.Error("D")
	}

	// Use the common pool.
	var wg sync.WaitGroup
	syncCalculateSum := func() {
		demo1()
		wg.Done()
	}
	for i := 0; i < runTimes; i++ {
		wg.Add(1)
		_ = ants.Submit(syncCalculateSum)
	}
	wg.Wait()
	fmt.Printf("running goroutines: %d\n", ants.Running())
	fmt.Printf("finish all tasks.\n")
}
