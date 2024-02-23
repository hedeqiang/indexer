// Copyright (c) 2023-2024 The UXUY Developer Team
// License:
// MIT License

// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:

// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.

// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE

package task

import (
	"github.com/uxuycom/indexer/config"
	"github.com/uxuycom/indexer/storage"
	"github.com/uxuycom/indexer/xylog"
)

type Task struct {
	dbc   *storage.DBClient
	cfg   *config.Config
	tasks map[string]interface{}
}

type ITask interface {
	Exec()
}

func InitTask(dbc *storage.DBClient, cfg *config.Config) *Task {

	task := &Task{
		tasks: map[string]interface{}{
			"chain_stats_tak": NewChainStatsTask(dbc, cfg), // add new task here
		},
	}

	for k, v := range task.tasks {
		xylog.Logger.Infof("tasks %v start!", k)
		t := v.(ITask)
		go t.Exec()
	}
	return task
}
