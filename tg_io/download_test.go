package tg_io

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"

	"github.com/gotd/td/middleware"
	"github.com/gotd/td/middleware/floodwait"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/dcs"
	"github.com/gotd/td/telegram/message"
	"github.com/gotd/td/telegram/uploader"
	"github.com/gotd/td/tg"

	"github.com/gotd/contrib/http_io"
	"github.com/gotd/contrib/middleware/floodwait"
	"github.com/gotd/contrib/partio"
)

const (
	chunk1kb = 1024
)

func TestE2E(t *testing.T) {
	if os.Getenv("TG_IO_E2E") != "1" {
		t.Skip("TG_IO_E2E not set")
	}
	logger := zaptest.NewLogger(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	client := telegram.NewClient(telegram.TestAppID, telegram.TestAppHash, telegram.Options{
		DC:     2,
		DCList: dcs.Staging(),
		Logger: logger.Named("client"),
		Middleware: middleware.Chain(
			floodwait.Middleware(),
		),
	})
	api := tg.NewClient(client)

	require.NoError(t, client.Run(ctx, func(ctx context.Context) error {
		if err := telegram.NewAuth(
			telegram.TestAuth(rand.Reader, 2),
			telegram.SendCodeOptions{},
		).Run(ctx, client.Auth()); err != nil {
			return err
		}

		const size = chunk1kb*5 + 100
		f, err := uploader.NewUploader(api).FromBytes(ctx, "upload.bin", make([]byte, size))
		if err != nil {
			return xerrors.Errorf("upload: %w", err)
		}

		mc, err := message.NewSender(api).Self().UploadMedia(ctx, message.File(f))
		if err != nil {
			return xerrors.Errorf("create media: %w", err)
		}

		media, ok := mc.(*tg.MessageMediaDocument)
		if !ok {
			return xerrors.Errorf("unexpected type: %T", media)
		}

		doc, ok := media.Document.AsNotEmpty()
		if !ok {
			return xerrors.Errorf("unexpected type: %T", media.Document)
		}

		t.Log("Streaming")
		u := partio.NewStreamer(NewDownloader(api).ChunkSource(doc.Size, doc.AsInputDocumentFileLocation()), chunk1kb)
		buf := new(bytes.Buffer)

		const offset = chunk1kb / 2
		if err := u.StreamAt(ctx, offset, buf); err != nil {
			return xerrors.Errorf("stream at %d: %w", offset, err)
		}

		t.Log(buf.Len())
		assert.Equal(t, doc.Size-offset, buf.Len())

		ln, err := net.Listen("tcp", "localhost:0")
		if err != nil {
			return xerrors.Errorf("listen: %w", err)
		}
		defer func() {
			_ = ln.Close()
		}()
		s := http.Server{
			Handler: http_io.NewHandler(u, doc.Size).
				WithContentType(doc.MimeType).
				WithLog(logger.Named("httpio")),
		}
		g, ctx := errgroup.WithContext(ctx)
		done := make(chan struct{})
		g.Go(func() error {
			select {
			case <-ctx.Done():
			case <-done:
			}
			return s.Close()
		})
		g.Go(func() error {
			if err := s.Serve(ln); err != nil && err != http.ErrServerClosed {
				return xerrors.Errorf("server: %w", err)
			}
			return nil
		})
		g.Go(func() error {
			defer close(done)

			requestURL := &url.URL{
				Scheme: "http",
				Host:   ln.Addr().String(),
			}
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL.String(), nil)
			if err != nil {
				return err
			}
			req.Header.Set("Range", fmt.Sprintf("bytes=%d-", offset))

			res, err := http.DefaultClient.Do(req)
			if err != nil {
				return xerrors.Errorf("send GET %q: %w", requestURL, err)
			}
			defer func() { _ = res.Body.Close() }()
			t.Log(res.Status)

			outBuf := new(bytes.Buffer)
			if _, err := io.Copy(outBuf, res.Body); err != nil {
				return xerrors.Errorf("read response: %w", err)
			}

			t.Log(outBuf.Len())

			return nil
		})

		return g.Wait()
	}))
}
