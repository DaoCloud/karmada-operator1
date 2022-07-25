/*
Copyright 2022 The Karmada operator Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package utils

import (
	"fmt"
	"io"

	"github.com/go-kit/kit/log"
)

// logWriter wraps a `log.Logger` so it can be used as an `io.Writer`.
type logWriter struct {
	log.Logger
}

// NewLogWriter returns an `io.Writer` for the given logger.
func NewLogWriter(logger log.Logger) io.Writer {
	return &logWriter{logger}
}

// Write simply logs the given byes as a 'write' operation, the only
// modification it makes before logging the given bytes is the removal
// of a terminating newline if present.
func (l *logWriter) Write(p []byte) (n int, err error) {
	origLen := len(p)
	if len(p) > 0 && p[len(p)-1] == '\n' {
		p = p[:len(p)-1] // Cut terminating newline
	}
	l.Log("info", fmt.Sprintf("%s", p))
	return origLen, nil
}
