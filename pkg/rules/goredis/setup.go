// Copyright (c) 2024 Alibaba Group Holding Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//go:build ignore

package goredis

import (
	"context"
	"net"
	"strings"

	redis "github.com/redis/go-redis/v9"
)

var goRedisInstrumenter = BuildGoRedisOtelInstrumenter()

func afterNewRedisClient(call redis.CallContext, client *redis.Client) {
	client.AddHook(newOtRedisHook(client.Options().Addr))
}

func afterNewFailOverRedisClient(call redis.CallContext, client *redis.Client) {
	client.AddHook(newOtRedisHook(client.Options().Addr))
}

func afterNewClusterClient(call redis.CallContext, client *redis.ClusterClient) {
	client.OnNewNode(func(rdb *redis.Client) {
		rdb.AddHook(newOtRedisHook(rdb.Options().Addr))
	})
}

func afterNewRingClient(call redis.CallContext, client *redis.Ring) {
	client.OnNewNode(func(rdb *redis.Client) {
		rdb.AddHook(newOtRedisHook(rdb.Options().Addr))
	})
}

func afterNewSentinelClient(call redis.CallContext, client *redis.SentinelClient) {
	client.AddHook(newOtRedisHook(client.String()))
}

func afterClientConn(call redis.CallContext, client *redis.Conn) {
	client.AddHook(newOtRedisHook(client.String()))
}

type otRedisHook struct {
	Addr string
}

func newOtRedisHook(addr string) *otRedisHook {
	return &otRedisHook{
		Addr: addr,
	}
}

func (o *otRedisHook) DialHook(next redis.DialHook) redis.DialHook {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		conn, err := next(ctx, network, addr)
		if err != nil {
			return nil, err
		}
		return conn, err
	}
}

func (o *otRedisHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		if strings.Contains(cmd.FullName(), "ping") || strings.Contains(cmd.FullName(), "PING") {
			return next(ctx, cmd)
		}
		request := goRedisRequest{
			cmd:      cmd,
			endpoint: o.Addr,
		}
		ctx = goRedisInstrumenter.Start(ctx, request)
		if err := next(ctx, cmd); err != nil {
			goRedisInstrumenter.End(ctx, request, nil, err)
			return err
		}
		goRedisInstrumenter.End(ctx, request, nil, nil)
		return nil
	}
}

func (o *otRedisHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []redis.Cmder) error {
		summary := ""
		summaryCmds := cmds
		if len(summaryCmds) > 10 {
			summaryCmds = summaryCmds[:10]
		}
		for i := range summaryCmds {
			summary += summaryCmds[i].FullName() + "/"
		}
		if len(cmds) > 10 {
			summary += "..."
		}
		cmd := redis.NewCmd(ctx, "pipeline", summary)
		request := goRedisRequest{
			cmd:      cmd,
			endpoint: o.Addr,
		}
		ctx = goRedisInstrumenter.Start(ctx, request)
		if err := next(ctx, cmds); err != nil {
			goRedisInstrumenter.End(ctx, request, nil, err)
			return err
		}
		goRedisInstrumenter.End(ctx, request, nil, nil)
		return nil
	}
}
