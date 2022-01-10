package etcd_test

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"time"

	"github.com/go-faster/errors"
	clientv3 "go.etcd.io/etcd/client/v3"

	tgauth "github.com/gotd/td/telegram/auth"

	"github.com/gotd/td/telegram"

	"github.com/gotd/contrib/auth"
	"github.com/gotd/contrib/auth/terminal"
	"github.com/gotd/contrib/etcd"
)

func etcdAuth(ctx context.Context) error {
	c, err := clientv3.New(clientv3.Config{
		Endpoints:   []string{"localhost:2379"},
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		return errors.Errorf("create etcd storage: %w", err)
	}
	cred := etcd.NewCredentials(c).
		WithPhoneKey("phone").
		WithPasswordKey("password")

	client, err := telegram.ClientFromEnvironment(telegram.Options{})
	if err != nil {
		return errors.Errorf("create client: %w", err)
	}

	return client.Run(ctx, func(ctx context.Context) error {
		return client.Auth().IfNecessary(
			ctx,
			tgauth.NewFlow(auth.Build(cred, terminal.OS()), tgauth.SendCodeOptions{}),
		)
	})
}

func ExampleCredentials() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	if err := etcdAuth(ctx); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "%+v\n", err)
		os.Exit(1)
	}
}
