package vault_test

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/hashicorp/vault/api"
	"golang.org/x/xerrors"

	"github.com/gotd/td/telegram"

	"github.com/gotd/contrib/auth"
	"github.com/gotd/contrib/auth/terminal"
	"github.com/gotd/contrib/auth/vault"
)

func vaultAuth(ctx context.Context) error {
	vaultClient, err := api.NewClient(api.DefaultConfig())
	if err != nil {
		return xerrors.Errorf("create Vault client: %w", err)
	}
	cred := vault.NewCredentials(vaultClient, "cubbyhole/telegram/user").
		WithPhoneKey("phone").
		WithPasswordKey("password")

	client, err := telegram.ClientFromEnvironment(telegram.Options{})
	if err != nil {
		return xerrors.Errorf("create client: %w", err)
	}

	return client.Run(ctx, func(ctx context.Context) error {
		return client.AuthIfNecessary(
			ctx,
			telegram.NewAuth(auth.Build(cred, terminal.OS()), telegram.SendCodeOptions{}),
		)
	})
}

func ExampleCredentials() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	if err := vaultAuth(ctx); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "%+v\n", err)
		os.Exit(1)
	}
}
