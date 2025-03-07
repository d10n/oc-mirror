package storage

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/remote/transport"
	"github.com/mholt/archiver/v3"
	"github.com/sirupsen/logrus"

	"github.com/openshift/oc-mirror/pkg/api/v1alpha2"
	"github.com/openshift/oc/pkg/cli/image/imagesource"
)

var _ Backend = &registryBackend{}

type registryBackend struct {
	// Since image contents are represented locally as directories,
	// use the local dir backend as the underlying Backend.
	*localDirBackend
	// Image to use when pushing and pulling
	src imagesource.TypedImageReference
	// Registry client options
	insecure bool
}

func NewRegistryBackend(cfg *v1alpha2.RegistryConfig, dir string) (Backend, error) {
	b := registryBackend{}
	b.insecure = cfg.SkipTLS

	ref, err := imagesource.ParseReference(cfg.ImageURL)
	if err != nil {
		return nil, err
	}
	if len(ref.Ref.Tag) == 0 {
		ref.Ref.Tag = "latest"
	}
	b.src = ref

	if b.localDirBackend == nil {
		// Create the local dir backend for local r/w.
		lb, err := NewLocalBackend(dir)
		if err != nil {
			return nil, fmt.Errorf("error creating local backend for registry: %w", err)
		}
		b.localDirBackend = lb.(*localDirBackend)
	}

	return &b, nil
}

// ReadMetadata unpacks the metadata image and read it from disk
func (b *registryBackend) ReadMetadata(ctx context.Context, meta *v1alpha2.Metadata, path string) error {
	logrus.Debugf("Checking for existing metadata image at %s", b.src)
	// Check if image exists
	if err := b.exists(ctx); err != nil {
		return err
	}
	if err := b.unpack(ctx, path); err != nil {
		return err
	}
	return b.localDirBackend.ReadMetadata(ctx, meta, path)
}

// WriteMetadata writes the provided metadata to disk anf registry.
func (b *registryBackend) WriteMetadata(ctx context.Context, meta *v1alpha2.Metadata, path string) error {
	return b.WriteObject(ctx, path, meta)
}

// ReadObject reads the provided object from disk.
// In this implementation, key is a file path.
func (b *registryBackend) ReadObject(ctx context.Context, fpath string, obj interface{}) error {
	return b.localDirBackend.ReadObject(ctx, fpath, obj)
}

// WriteObject writes the provided object to disk and registry.
// In this implementation, key is a file path.
func (b *registryBackend) WriteObject(ctx context.Context, fpath string, obj interface{}) (err error) {
	var data []byte
	switch v := obj.(type) {
	case []byte:
		data = v
	case string:
		data = []byte(v)
	case io.Reader:
		data, err = io.ReadAll(v)
	default:
		data, err = json.Marshal(obj)
	}
	if err != nil {
		return err
	}

	// Write metadata to disk for packing into archive
	if err := b.localDirBackend.WriteObject(ctx, fpath, obj); err != nil {
		return err
	}
	logrus.Debugf("Pushing metadata to registry at %s", b.src)
	return b.pushImage(ctx, data, fpath)
}

// GetWriter returns an os.File as a writer.
// In this implementation, key is a file path.
func (b *registryBackend) GetWriter(ctx context.Context, fpath string) (io.Writer, error) {
	return b.localDirBackend.GetWriter(ctx, fpath)
}

// Open reads the provided object from a registry source and provides an io.ReadCloser
func (b *registryBackend) Open(ctx context.Context, fpath string) (io.ReadCloser, error) {
	if _, err := b.Stat(ctx, fpath); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		if err := b.unpack(ctx, fpath); err != nil {
			return nil, err
		}
	}
	return b.localDirBackend.Open(ctx, fpath)
}

func (b *registryBackend) unpack(ctx context.Context, fpath string) error {
	tempTar := fmt.Sprintf("%s.tar", b.src.Ref.Name)
	opts := b.getOpts(ctx)
	img, err := crane.Pull(b.src.Ref.Exact(), opts...)
	if err != nil {
		return err
	}
	w, err := b.GetWriter(ctx, tempTar)
	if err != nil {
		return err
	}
	defer b.localDirBackend.fs.Remove(tempTar)

	if err := crane.Export(img, w); err != nil {
		return err
	}
	arc := archiver.Tar{
		OverwriteExisting:      true,
		MkdirAll:               true,
		ImplicitTopLevelFolder: false,
		StripComponents:        0,
		ContinueOnError:        false,
	}
	if err := arc.Unarchive(filepath.Join(b.localDirBackend.dir, tempTar), b.localDirBackend.dir); err != nil {
		return err
	} // adjust perms, unpack leaves the file user-writable only
	return b.localDirBackend.fs.Chmod(fpath, 0600)
}

// Stat checks the existence of the metadata from a registry source
func (b *registryBackend) Stat(ctx context.Context, fpath string) (os.FileInfo, error) {
	logrus.Debugf("Checking for existing metadata image at %s", b.src.String())
	// Check if image exists
	if err := b.exists(ctx); err != nil {
		return nil, err
	}
	return b.localDirBackend.Stat(ctx, fpath)
}

// Cleanup removes metadata from existing metadata from backend location
func (b *registryBackend) Cleanup(ctx context.Context, fpath string) error {
	opts := b.getOpts(ctx)
	if err := crane.Delete(b.src.Ref.Exact(), opts...); err != nil {
		return err
	}
	return b.localDirBackend.Cleanup(ctx, fpath)
}

// CheckConfig will return an error if the StorageConfig
// is not a registry
func (b *registryBackend) CheckConfig(storage v1alpha2.StorageConfig) error {
	if storage.Registry == nil {
		return fmt.Errorf("not registry backend")
	}
	return nil
}

// pushImage will push a v1.Image with provided contents
func (b *registryBackend) pushImage(ctx context.Context, data []byte, fpath string) error {
	opts := b.getOpts(ctx)
	contents := map[string][]byte{
		fpath: data,
	}
	i, _ := crane.Image(contents)
	return crane.Push(i, b.src.Ref.Exact(), opts...)
}

// exists checks if the image exists
func (b *registryBackend) exists(ctx context.Context) error {
	opts := b.getOpts(ctx)
	_, err := crane.Manifest(b.src.Ref.Exact(), opts...)
	var terr *transport.Error
	switch {
	case err != nil && errors.As(err, &terr) && terr.StatusCode == 404:
		return ErrMetadataNotExist
	case err != nil && errors.As(err, &terr) && terr.StatusCode == 401:
		var nameOpts []name.Option
		if b.insecure {
			nameOpts = append(nameOpts, name.Insecure)
		}
		ref, err := name.ParseReference(b.src.Ref.Exact(), nameOpts...)
		if err != nil {
			return err
		}
		err = remote.CheckPushPermission(ref, authn.DefaultKeychain, b.createRT())
		if err != nil {
			return err
		}
		// return metadata does not exist
		// if push permission does not throw an error
		return ErrMetadataNotExist
	default:
		return err
	}
}

func (b *registryBackend) createRT() http.RoundTripper {
	return &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			// By default we wrap the transport in retries, so reduce the
			// default dial timeout to 5s to avoid 5x 30s of connection
			// timeouts when doing the "ping" on certain http registries.
			Timeout:   5 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: b.insecure,
		},
	}
}

// TODO: Get default auth will need to update if user
// can specify custom locations
func (b *registryBackend) getOpts(ctx context.Context) []crane.Option {
	options := []crane.Option{
		crane.WithAuthFromKeychain(authn.DefaultKeychain),
		crane.WithContext(ctx),
		crane.WithTransport(b.createRT()),
	}
	if b.insecure {
		options = append(options, crane.Insecure)
	}
	return options
}
