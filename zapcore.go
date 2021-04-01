package zaplogman

import (
	"time"

	"go.uber.org/zap/zapcore"
)

type core struct {
	man *Man
	enc zapcore.Encoder
}

func NewCore(enc zapcore.Encoder, man *Man) zapcore.Core {
	man.logfiles = make(map[string]*logfile)
	man.filemap = make(map[time.Time]string)
	return &core{
		man: man,
		enc: enc,
	}
}

func (c *core) With(fields []zapcore.Field) zapcore.Core {
	// clone := c.clone()
	for i := range fields {
		fields[i].AddTo(c.enc)
	}
	return c
}

func (c *core) Enabled(lvl zapcore.Level) bool {
	return c.man.Level <= lvl
}

func (c *core) Check(ent zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if c.Enabled(ent.Level) {
		return ce.AddCore(ent, c)
	}
	return ce
}

func (c *core) Write(ent zapcore.Entry, fields []zapcore.Field) error {
	buf, err := c.enc.EncodeEntry(ent, fields)
	if err != nil {
		return err
	}

	err = c.man.dispatch(ent.Time, buf.Bytes())
	buf.Free()
	if err != nil {
		return err
	}
	return nil
}

func (c *core) Sync() error {
	c.man.waitAll()
	return nil
}

func (c *core) clone() *core {
	return &core{
		man: c.man,
		enc: c.enc.Clone(),
	}
}
