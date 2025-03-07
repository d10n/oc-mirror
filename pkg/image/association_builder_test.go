package image

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/docker/distribution/manifest"
	"github.com/openshift/library-go/pkg/image/reference"
	"github.com/openshift/oc-mirror/pkg/api/v1alpha2"
	"github.com/openshift/oc/pkg/cli/image/imagesource"
	"github.com/stretchr/testify/require"
)

func TestAssociateLocalImageLayers(t *testing.T) {
	tests := []struct {
		name       string
		imgTyp     v1alpha2.ImageType
		imgMapping TypedImageMapping
		expResult  AssociationSet
		expError   error
		wantErr    bool
	}{
		{
			name:   "Valid/ManifestWithTag",
			imgTyp: v1alpha2.TypeGeneric,
			imgMapping: map[TypedImage]TypedImage{
				{
					TypedImageReference: imagesource.TypedImageReference{
						Ref: reference.DockerImageReference{
							Name: "imgname",
							Tag:  "latest",
						}},
					Category: v1alpha2.TypeGeneric}: {
					TypedImageReference: imagesource.TypedImageReference{
						Ref: reference.DockerImageReference{
							Name: "single_manifest",
							Tag:  "latest",
						},
						Type: imagesource.DestinationFile,
					},
					Category: v1alpha2.TypeGeneric}},
			expResult: AssociationSet{"imgname:latest": Associations{
				"imgname:latest": {
					Name:            "imgname:latest",
					Path:            "single_manifest",
					TagSymlink:      "latest",
					ID:              "sha256:d31c6ea5c50be93d6eb94d2b508f0208e84a308c011c6454ebf291d48b37df19",
					Type:            v1alpha2.TypeGeneric,
					ManifestDigests: nil,
					LayerDigests: []string{
						"sha256:e8614d09b7bebabd9d8a450f44e88a8807c98a438a2ddd63146865286b132d1b",
						"sha256:601401253d0aac2bc95cccea668761a6e69216468809d1cee837b2e8b398e241",
						"sha256:211941188a4f55ffc6bcefa4f69b69b32c13fafb65738075de05808bbfcec086",
						"sha256:f0fd5be261dfd2e36d01069a387a3e5125f5fd5adfec90f3cb190d1d5f1d1ad9",
						"sha256:0c0beb258254c0566315c641b4107b080a96fa78d4f96833453dd6c5b9edf2b7",
						"sha256:30c794a11b4c340c77238c5b7ca845752904bd8b74b73a9b16d31253234da031",
					},
				},
			}},
		},
		{
			name:   "Valid/ManifestWithDigest",
			imgTyp: v1alpha2.TypeGeneric,
			imgMapping: map[TypedImage]TypedImage{
				{
					TypedImageReference: imagesource.TypedImageReference{
						Ref: reference.DockerImageReference{
							Name: "imgname",
							ID:   "sha256:d31c6ea5c50be93d6eb94d2b508f0208e84a308c011c6454ebf291d48b37df19",
						}},
					Category: v1alpha2.TypeGeneric}: {
					TypedImageReference: imagesource.TypedImageReference{
						Ref: reference.DockerImageReference{
							Name: "single_manifest",
							ID:   "sha256:d31c6ea5c50be93d6eb94d2b508f0208e84a308c011c6454ebf291d48b37df19",
						},
						Type: imagesource.DestinationFile,
					},
					Category: v1alpha2.TypeGeneric}},
			expResult: AssociationSet{"imgname@sha256:d31c6ea5c50be93d6eb94d2b508f0208e84a308c011c6454ebf291d48b37df19": Associations{
				"imgname@sha256:d31c6ea5c50be93d6eb94d2b508f0208e84a308c011c6454ebf291d48b37df19": {
					Name:            "imgname@sha256:d31c6ea5c50be93d6eb94d2b508f0208e84a308c011c6454ebf291d48b37df19",
					Path:            "single_manifest",
					TagSymlink:      "oc-mirrord31c6e",
					ID:              "sha256:d31c6ea5c50be93d6eb94d2b508f0208e84a308c011c6454ebf291d48b37df19",
					Type:            v1alpha2.TypeGeneric,
					ManifestDigests: nil,
					LayerDigests: []string{
						"sha256:e8614d09b7bebabd9d8a450f44e88a8807c98a438a2ddd63146865286b132d1b",
						"sha256:601401253d0aac2bc95cccea668761a6e69216468809d1cee837b2e8b398e241",
						"sha256:211941188a4f55ffc6bcefa4f69b69b32c13fafb65738075de05808bbfcec086",
						"sha256:f0fd5be261dfd2e36d01069a387a3e5125f5fd5adfec90f3cb190d1d5f1d1ad9",
						"sha256:0c0beb258254c0566315c641b4107b080a96fa78d4f96833453dd6c5b9edf2b7",
						"sha256:30c794a11b4c340c77238c5b7ca845752904bd8b74b73a9b16d31253234da031",
					},
				},
			}},
		},
		{
			name:   "Valid/IndexManifest",
			imgTyp: v1alpha2.TypeGeneric,
			imgMapping: map[TypedImage]TypedImage{
				{
					TypedImageReference: imagesource.TypedImageReference{
						Ref: reference.DockerImageReference{
							Name: "imgname",
							Tag:  "latest",
						}},
					Category: v1alpha2.TypeGeneric}: {
					TypedImageReference: imagesource.TypedImageReference{
						Ref: reference.DockerImageReference{
							Name: "index_manifest",
							Tag:  "latest",
						},
						Type: imagesource.DestinationFile,
					},
					Category: v1alpha2.TypeGeneric}},
			expResult: AssociationSet{"imgname:latest": Associations{
				"imgname:latest": {
					Name:       "imgname:latest",
					Path:       "index_manifest",
					TagSymlink: "latest",
					ID:         "sha256:d15a206e4ee462e82ab722ed84dfa514ab9ed8d85100d591c04314ae7c2162ee",
					Type:       v1alpha2.TypeGeneric,
					ManifestDigests: []string{
						"sha256:bab3a6153010b614c8764548f0dbe34c4a7dce4ea278a94713c3e9a936bb74e6",
						"sha256:9574416689665a82cb4eaf43463da5b6156071ebbec117262eef7fa32b4d7021",
						"sha256:b8a825862d73b2f1110dd9c5fc0631f47117c7cd99e42efa34244cd82bd6742f",
						"sha256:60f5921e0f6a21a485a0a4e9415761afb5b60814bbe8a6864cb12b90ae24c1d0",
					},
					LayerDigests: nil,
				},
				"sha256:60f5921e0f6a21a485a0a4e9415761afb5b60814bbe8a6864cb12b90ae24c1d0": {
					Name:       "sha256:60f5921e0f6a21a485a0a4e9415761afb5b60814bbe8a6864cb12b90ae24c1d0",
					Path:       "index_manifest",
					TagSymlink: "",
					ID:         "sha256:60f5921e0f6a21a485a0a4e9415761afb5b60814bbe8a6864cb12b90ae24c1d0",
					Type:       v1alpha2.TypeGeneric,
					LayerDigests: []string{
						"sha256:b538f80385f9b48122e3da068c932a96ea5018afa3c7be79da00437414bd18cd",
						"sha256:342a15c43afd15b4d93051022ecf020ea6fde1e14d34599f5b4c10a8a5bae3c6",
						"sha256:70660e39ee11b715823a96729d7f1b8964ecd6ca2b7c0e3fd5cde284e34758eb",
						"sha256:f553d3748799c35aa60227875706f727a526a1d4c7840a5d550cdb4ba6cd5196",
						"sha256:c5338ca295456f5c677bf8910ac94765be2f53977af6bd792f18a2298054d6be",
						"sha256:af94dd630ca5e3e15d15502c2a03e386f4c1ef5a59def62e84ede35a009c4110",
						"sha256:337fc839f463fd6b6d1773e0b8f2f9d40b3a8dff6963008193344cd29466a3d1",
						"sha256:4d4b85daa42ca075d8aff8563d14434799268a4b823e74737171ed438f8c60ad",
					},
				},
				"sha256:9574416689665a82cb4eaf43463da5b6156071ebbec117262eef7fa32b4d7021": {
					Name:       "sha256:9574416689665a82cb4eaf43463da5b6156071ebbec117262eef7fa32b4d7021",
					Path:       "index_manifest",
					TagSymlink: "",
					ID:         "sha256:9574416689665a82cb4eaf43463da5b6156071ebbec117262eef7fa32b4d7021",
					Type:       v1alpha2.TypeGeneric,
					LayerDigests: []string{
						"sha256:b4b72e716706d29f5d2351709c20bf737b94f876a5472a43ff1b6e203c65d27f",
						"sha256:8d0157f7a4ed4136f430f737f0f79d650248e19ebd87371f1ae1735536f0eaf2",
						"sha256:46f9bc09f2ae8c0a95a69d77cd91527281cf54cd466dbee5ba6b28e05ee68a77",
						"sha256:21d0f0a83af189ace4e566f1520e8ac5a404adda15edb534ee79a994bdd94abe",
						"sha256:61a5adb16b8c308ed6481d3abac7e08035f09d936f2a1ecad0bd2000a18464b9",
						"sha256:a92dcc7bd9c9c1369ef92728f7649e3ec868b53b7b38ab2a4bddc525f74896a8",
						"sha256:317a9dc239a3310e2010e6e1c4f2a87b4b2c53f49ca5231c031227540ef91d0b",
						"sha256:d476ce7797cc1558919a31a1cccd9b09f48ea2787982ccd3c2576252450d2d51",
					},
				},
				"sha256:b8a825862d73b2f1110dd9c5fc0631f47117c7cd99e42efa34244cd82bd6742f": {
					Name:       "sha256:b8a825862d73b2f1110dd9c5fc0631f47117c7cd99e42efa34244cd82bd6742f",
					Path:       "index_manifest",
					TagSymlink: "",
					ID:         "sha256:b8a825862d73b2f1110dd9c5fc0631f47117c7cd99e42efa34244cd82bd6742f",
					Type:       v1alpha2.TypeGeneric,
					LayerDigests: []string{
						"sha256:52278dd8e57993669c5b72a9620e89bebdc098f2af2379caaa8945f7403f77a2",
						"sha256:1dc2a2c4dd124cf83f27e6d8852303f7874507b71a3f7b6a1265837b43279092",
						"sha256:26100ac97b3237b89768d0dac0150c6a2b483a16b0662160df98d03ba25fa474",
						"sha256:7c120a97d24392c377b955ca42f09fc04942aecff3f0a007d31ebd20c185958a",
						"sha256:87875760340f78f13107842911184c55308475062940399772e7944138879704",
						"sha256:5ad5a4942ddf238ce385d4b29eaa3b2d5f8836de538918d7da9a839c8313fd46",
						"sha256:6121cb3c461255702c8b8ef03ed4b13061c0c600b20c7664ce82815ed15febbd",
						"sha256:c72bf53b697715cd03c3f3dc6fd6d2bccb4b10e511c2847eb98e312d28850e48",
					},
				},
				"sha256:bab3a6153010b614c8764548f0dbe34c4a7dce4ea278a94713c3e9a936bb74e6": {
					Name:       "sha256:bab3a6153010b614c8764548f0dbe34c4a7dce4ea278a94713c3e9a936bb74e6",
					Path:       "index_manifest",
					TagSymlink: "",
					ID:         "sha256:bab3a6153010b614c8764548f0dbe34c4a7dce4ea278a94713c3e9a936bb74e6",
					Type:       v1alpha2.TypeGeneric,
					LayerDigests: []string{
						"sha256:df20fa9351a15782c64e6dddb2d4a6f50bf6d3688060a34c4014b0d9a752eb4c",
						"sha256:58445347cff86791f89717f3bf79ec6f597d146397d9e78136cf9e937f363555",
						"sha256:49f791cfca3e59c6094ec94d091473ddd9fe206e9860c0eb37dacbc3bbcccafd",
						"sha256:b83c8811a2df5586918135a8bab5304c9c6f0c0a3b103c4b3ceb4515d2c480a5",
						"sha256:36821795adb1d93e34b9835d2cd738738e0a7fb99b6232f00f69a0146f6db7fa",
						"sha256:f31bf23bf137d6210ce78d1b133bab25ae0daffda0bfff172476479dfcc0b3a1",
						"sha256:59064015f738a38367ca0ef7083840f3f1dbc579aa208071b4fb6b022a48d89a",
						"sha256:3f161edc88f5ebe6db761902c3e563f450a8f373f58f6f9f59a13a7954f57d90",
					},
				},
			}},
		},
		{
			name:   "Invalid/InvalidComponent",
			imgTyp: v1alpha2.TypeGeneric,
			imgMapping: map[TypedImage]TypedImage{
				{
					TypedImageReference: imagesource.TypedImageReference{
						Ref: reference.DockerImageReference{
							Name: "imgname",
						}},
					Category: v1alpha2.TypeGeneric}: {
					TypedImageReference: imagesource.TypedImageReference{
						Ref: reference.DockerImageReference{
							Name: "single_manifest",
						},
						Type: imagesource.DestinationFile,
					},
					Category: v1alpha2.TypeGeneric}},
			wantErr:  true,
			expError: &ErrInvalidComponent{},
		},
		{
			name:   "Invalid/MissingImage",
			imgTyp: v1alpha2.TypeGeneric,
			imgMapping: map[TypedImage]TypedImage{
				{
					TypedImageReference: imagesource.TypedImageReference{
						Ref: reference.DockerImageReference{
							Name: "imgname",
							Tag:  "latest",
						}},
					Category: v1alpha2.TypeGeneric}: {
					TypedImageReference: imagesource.TypedImageReference{
						Ref: reference.DockerImageReference{
							Name: "fake_manifest",
							Tag:  "latest",
						},
						Type: imagesource.DestinationFile,
					},
					Category: v1alpha2.TypeGeneric}},
			wantErr:  true,
			expError: &ErrInvalidImage{},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tmpdir := t.TempDir()
			require.NoError(t, copyV2("testdata", tmpdir))
			asSet, err := AssociateLocalImageLayers(tmpdir, test.imgMapping)
			if !test.wantErr {
				require.NoError(t, err)
				require.Equal(t, test.expResult, asSet)
			} else {
				require.ErrorAs(t, err, &test.expError)
			}
		})
	}
}

func TestAssociateRemoteImageLayers(t *testing.T) {

	server := httptest.NewServer(mirrorV2("testdata"))
	t.Cleanup(server.Close)
	u, err := url.Parse(server.URL)
	require.NoError(t, err)

	tests := []struct {
		name       string
		imgTyp     v1alpha2.ImageType
		imgMapping TypedImageMapping
		expResult  AssociationSet
		expError   error
		wantErr    bool
	}{
		{
			name:   "Valid/ManifestWithTag",
			imgTyp: v1alpha2.TypeGeneric,
			imgMapping: map[TypedImage]TypedImage{
				{
					TypedImageReference: imagesource.TypedImageReference{
						Ref: reference.DockerImageReference{
							Name:     "single_manifest",
							Tag:      "latest",
							Registry: u.Host,
						}},
					Category: v1alpha2.TypeGeneric}: {
					TypedImageReference: imagesource.TypedImageReference{
						Ref: reference.DockerImageReference{
							Name:     "single_manifest",
							Tag:      "latest",
							Registry: "test-registry",
						},
						Type: imagesource.DestinationRegistry,
					},
					Category: v1alpha2.TypeGeneric}},
			expResult: AssociationSet{fmt.Sprintf("%s/single_manifest@sha256:d31c6ea5c50be93d6eb94d2b508f0208e84a308c011c6454ebf291d48b37df19", u.Host): Associations{
				fmt.Sprintf("%s/single_manifest@sha256:d31c6ea5c50be93d6eb94d2b508f0208e84a308c011c6454ebf291d48b37df19", u.Host): {
					Name:            fmt.Sprintf("%s/single_manifest@sha256:d31c6ea5c50be93d6eb94d2b508f0208e84a308c011c6454ebf291d48b37df19", u.Host),
					Path:            "test-registry/single_manifest:latest",
					TagSymlink:      "latest",
					ID:              "sha256:d31c6ea5c50be93d6eb94d2b508f0208e84a308c011c6454ebf291d48b37df19",
					Type:            v1alpha2.TypeGeneric,
					ManifestDigests: nil,
					LayerDigests: []string{
						"sha256:e8614d09b7bebabd9d8a450f44e88a8807c98a438a2ddd63146865286b132d1b",
						"sha256:601401253d0aac2bc95cccea668761a6e69216468809d1cee837b2e8b398e241",
						"sha256:211941188a4f55ffc6bcefa4f69b69b32c13fafb65738075de05808bbfcec086",
						"sha256:f0fd5be261dfd2e36d01069a387a3e5125f5fd5adfec90f3cb190d1d5f1d1ad9",
						"sha256:0c0beb258254c0566315c641b4107b080a96fa78d4f96833453dd6c5b9edf2b7",
						"sha256:30c794a11b4c340c77238c5b7ca845752904bd8b74b73a9b16d31253234da031",
					},
				},
			}},
		},
		{
			name:   "Valid/ManifestWithDigest",
			imgTyp: v1alpha2.TypeGeneric,
			imgMapping: map[TypedImage]TypedImage{
				{
					TypedImageReference: imagesource.TypedImageReference{
						Ref: reference.DockerImageReference{
							Name:     "single_manifest",
							ID:       "sha256:d31c6ea5c50be93d6eb94d2b508f0208e84a308c011c6454ebf291d48b37df19",
							Tag:      "latest",
							Registry: u.Host,
						}},
					Category: v1alpha2.TypeGeneric}: {
					TypedImageReference: imagesource.TypedImageReference{
						Ref: reference.DockerImageReference{
							Name:     "single_manifest",
							ID:       "sha256:d31c6ea5c50be93d6eb94d2b508f0208e84a308c011c6454ebf291d48b37df19",
							Registry: "test-registry",
						},
						Type: imagesource.DestinationRegistry,
					},
					Category: v1alpha2.TypeGeneric}},
			expResult: AssociationSet{fmt.Sprintf("%s/single_manifest@sha256:d31c6ea5c50be93d6eb94d2b508f0208e84a308c011c6454ebf291d48b37df19", u.Host): Associations{
				fmt.Sprintf("%s/single_manifest@sha256:d31c6ea5c50be93d6eb94d2b508f0208e84a308c011c6454ebf291d48b37df19", u.Host): {
					Name:            fmt.Sprintf("%s/single_manifest@sha256:d31c6ea5c50be93d6eb94d2b508f0208e84a308c011c6454ebf291d48b37df19", u.Host),
					Path:            "test-registry/single_manifest@sha256:d31c6ea5c50be93d6eb94d2b508f0208e84a308c011c6454ebf291d48b37df19",
					TagSymlink:      "latest",
					ID:              "sha256:d31c6ea5c50be93d6eb94d2b508f0208e84a308c011c6454ebf291d48b37df19",
					Type:            v1alpha2.TypeGeneric,
					ManifestDigests: nil,
					LayerDigests: []string{
						"sha256:e8614d09b7bebabd9d8a450f44e88a8807c98a438a2ddd63146865286b132d1b",
						"sha256:601401253d0aac2bc95cccea668761a6e69216468809d1cee837b2e8b398e241",
						"sha256:211941188a4f55ffc6bcefa4f69b69b32c13fafb65738075de05808bbfcec086",
						"sha256:f0fd5be261dfd2e36d01069a387a3e5125f5fd5adfec90f3cb190d1d5f1d1ad9",
						"sha256:0c0beb258254c0566315c641b4107b080a96fa78d4f96833453dd6c5b9edf2b7",
						"sha256:30c794a11b4c340c77238c5b7ca845752904bd8b74b73a9b16d31253234da031",
					},
				},
			}},
		},
		{
			name:   "Valid/IndexManifest",
			imgTyp: v1alpha2.TypeGeneric,
			imgMapping: map[TypedImage]TypedImage{
				{
					TypedImageReference: imagesource.TypedImageReference{
						Ref: reference.DockerImageReference{
							Name:     "index_manifest",
							Tag:      "latest",
							ID:       "sha256:d15a206e4ee462e82ab722ed84dfa514ab9ed8d85100d591c04314ae7c2162ee",
							Registry: u.Host,
						}},
					Category: v1alpha2.TypeGeneric}: {
					TypedImageReference: imagesource.TypedImageReference{
						Ref: reference.DockerImageReference{
							Name:     "index_manifest",
							Tag:      "latest",
							Registry: "test-registry",
						},
						Type: imagesource.DestinationRegistry,
					},
					Category: v1alpha2.TypeGeneric}},
			expResult: AssociationSet{fmt.Sprintf("%s/index_manifest@sha256:d15a206e4ee462e82ab722ed84dfa514ab9ed8d85100d591c04314ae7c2162ee", u.Host): Associations{
				fmt.Sprintf("%s/index_manifest@sha256:d15a206e4ee462e82ab722ed84dfa514ab9ed8d85100d591c04314ae7c2162ee", u.Host): {
					Name:       fmt.Sprintf("%s/index_manifest@sha256:d15a206e4ee462e82ab722ed84dfa514ab9ed8d85100d591c04314ae7c2162ee", u.Host),
					Path:       "test-registry/index_manifest:latest",
					TagSymlink: "latest",
					ID:         "sha256:d15a206e4ee462e82ab722ed84dfa514ab9ed8d85100d591c04314ae7c2162ee",
					Type:       v1alpha2.TypeGeneric,
					ManifestDigests: []string{
						"sha256:bab3a6153010b614c8764548f0dbe34c4a7dce4ea278a94713c3e9a936bb74e6",
						"sha256:9574416689665a82cb4eaf43463da5b6156071ebbec117262eef7fa32b4d7021",
						"sha256:b8a825862d73b2f1110dd9c5fc0631f47117c7cd99e42efa34244cd82bd6742f",
						"sha256:60f5921e0f6a21a485a0a4e9415761afb5b60814bbe8a6864cb12b90ae24c1d0",
					},
					LayerDigests: nil,
				},
				"sha256:60f5921e0f6a21a485a0a4e9415761afb5b60814bbe8a6864cb12b90ae24c1d0": {
					Name:       "sha256:60f5921e0f6a21a485a0a4e9415761afb5b60814bbe8a6864cb12b90ae24c1d0",
					Path:       "test-registry/index_manifest:latest",
					TagSymlink: "",
					ID:         "sha256:60f5921e0f6a21a485a0a4e9415761afb5b60814bbe8a6864cb12b90ae24c1d0",
					Type:       v1alpha2.TypeGeneric,
					LayerDigests: []string{
						"sha256:b538f80385f9b48122e3da068c932a96ea5018afa3c7be79da00437414bd18cd",
						"sha256:342a15c43afd15b4d93051022ecf020ea6fde1e14d34599f5b4c10a8a5bae3c6",
						"sha256:70660e39ee11b715823a96729d7f1b8964ecd6ca2b7c0e3fd5cde284e34758eb",
						"sha256:f553d3748799c35aa60227875706f727a526a1d4c7840a5d550cdb4ba6cd5196",
						"sha256:c5338ca295456f5c677bf8910ac94765be2f53977af6bd792f18a2298054d6be",
						"sha256:af94dd630ca5e3e15d15502c2a03e386f4c1ef5a59def62e84ede35a009c4110",
						"sha256:337fc839f463fd6b6d1773e0b8f2f9d40b3a8dff6963008193344cd29466a3d1",
						"sha256:4d4b85daa42ca075d8aff8563d14434799268a4b823e74737171ed438f8c60ad",
					},
				},
				"sha256:9574416689665a82cb4eaf43463da5b6156071ebbec117262eef7fa32b4d7021": {
					Name:       "sha256:9574416689665a82cb4eaf43463da5b6156071ebbec117262eef7fa32b4d7021",
					Path:       "test-registry/index_manifest:latest",
					TagSymlink: "",
					ID:         "sha256:9574416689665a82cb4eaf43463da5b6156071ebbec117262eef7fa32b4d7021",
					Type:       v1alpha2.TypeGeneric,
					LayerDigests: []string{
						"sha256:b4b72e716706d29f5d2351709c20bf737b94f876a5472a43ff1b6e203c65d27f",
						"sha256:8d0157f7a4ed4136f430f737f0f79d650248e19ebd87371f1ae1735536f0eaf2",
						"sha256:46f9bc09f2ae8c0a95a69d77cd91527281cf54cd466dbee5ba6b28e05ee68a77",
						"sha256:21d0f0a83af189ace4e566f1520e8ac5a404adda15edb534ee79a994bdd94abe",
						"sha256:61a5adb16b8c308ed6481d3abac7e08035f09d936f2a1ecad0bd2000a18464b9",
						"sha256:a92dcc7bd9c9c1369ef92728f7649e3ec868b53b7b38ab2a4bddc525f74896a8",
						"sha256:317a9dc239a3310e2010e6e1c4f2a87b4b2c53f49ca5231c031227540ef91d0b",
						"sha256:d476ce7797cc1558919a31a1cccd9b09f48ea2787982ccd3c2576252450d2d51",
					},
				},
				"sha256:b8a825862d73b2f1110dd9c5fc0631f47117c7cd99e42efa34244cd82bd6742f": {
					Name:       "sha256:b8a825862d73b2f1110dd9c5fc0631f47117c7cd99e42efa34244cd82bd6742f",
					Path:       "test-registry/index_manifest:latest",
					TagSymlink: "",
					ID:         "sha256:b8a825862d73b2f1110dd9c5fc0631f47117c7cd99e42efa34244cd82bd6742f",
					Type:       v1alpha2.TypeGeneric,
					LayerDigests: []string{
						"sha256:52278dd8e57993669c5b72a9620e89bebdc098f2af2379caaa8945f7403f77a2",
						"sha256:1dc2a2c4dd124cf83f27e6d8852303f7874507b71a3f7b6a1265837b43279092",
						"sha256:26100ac97b3237b89768d0dac0150c6a2b483a16b0662160df98d03ba25fa474",
						"sha256:7c120a97d24392c377b955ca42f09fc04942aecff3f0a007d31ebd20c185958a",
						"sha256:87875760340f78f13107842911184c55308475062940399772e7944138879704",
						"sha256:5ad5a4942ddf238ce385d4b29eaa3b2d5f8836de538918d7da9a839c8313fd46",
						"sha256:6121cb3c461255702c8b8ef03ed4b13061c0c600b20c7664ce82815ed15febbd",
						"sha256:c72bf53b697715cd03c3f3dc6fd6d2bccb4b10e511c2847eb98e312d28850e48",
					},
				},
				"sha256:bab3a6153010b614c8764548f0dbe34c4a7dce4ea278a94713c3e9a936bb74e6": {
					Name:       "sha256:bab3a6153010b614c8764548f0dbe34c4a7dce4ea278a94713c3e9a936bb74e6",
					Path:       "test-registry/index_manifest:latest",
					TagSymlink: "",
					ID:         "sha256:bab3a6153010b614c8764548f0dbe34c4a7dce4ea278a94713c3e9a936bb74e6",
					Type:       v1alpha2.TypeGeneric,
					LayerDigests: []string{
						"sha256:df20fa9351a15782c64e6dddb2d4a6f50bf6d3688060a34c4014b0d9a752eb4c",
						"sha256:58445347cff86791f89717f3bf79ec6f597d146397d9e78136cf9e937f363555",
						"sha256:49f791cfca3e59c6094ec94d091473ddd9fe206e9860c0eb37dacbc3bbcccafd",
						"sha256:b83c8811a2df5586918135a8bab5304c9c6f0c0a3b103c4b3ceb4515d2c480a5",
						"sha256:36821795adb1d93e34b9835d2cd738738e0a7fb99b6232f00f69a0146f6db7fa",
						"sha256:f31bf23bf137d6210ce78d1b133bab25ae0daffda0bfff172476479dfcc0b3a1",
						"sha256:59064015f738a38367ca0ef7083840f3f1dbc579aa208071b4fb6b022a48d89a",
						"sha256:3f161edc88f5ebe6db761902c3e563f450a8f373f58f6f9f59a13a7954f57d90",
					},
				},
			}},
		},
		{
			name:   "Invalid/InvalidComponent",
			imgTyp: v1alpha2.TypeGeneric,
			imgMapping: map[TypedImage]TypedImage{
				{
					TypedImageReference: imagesource.TypedImageReference{
						Ref: reference.DockerImageReference{
							Name: "imgname",
						}},
					Category: v1alpha2.TypeGeneric}: {
					TypedImageReference: imagesource.TypedImageReference{
						Ref: reference.DockerImageReference{
							Name: "single_manifest",
						},
						Type: imagesource.DestinationRegistry,
					},
					Category: v1alpha2.TypeGeneric}},
			wantErr:  true,
			expError: &ErrInvalidComponent{},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			asSet, err := AssociateRemoteImageLayers(context.TODO(), test.imgMapping, true, true, false)
			if !test.wantErr {
				require.NoError(t, err)
				require.Equal(t, test.expResult, asSet)
			} else {
				require.ErrorAs(t, err, &test.expError)
			}
		})
	}
}

func mirrorV2(v2Dir string) http.HandlerFunc {
	dir := http.Dir(v2Dir)
	fileHandler := http.FileServer(dir)
	handler := func(w http.ResponseWriter, req *http.Request) {
		if req.Method == "GET" && req.URL.Path == "/v2/" {
			w.Header().Set("Docker-Distribution-API-Version", "2.0")
		}
		if req.Method == "GET" {
			switch path.Base(path.Dir(req.URL.Path)) {
			case "blobs":
				w.Header().Set("Content-Type", "application/octet-stream")
			case "manifests":
				if f, err := dir.Open(req.URL.Path); err == nil {
					defer f.Close()
					if data, err := ioutil.ReadAll(f); err == nil {
						var versioned manifest.Versioned
						if err = json.Unmarshal(data, &versioned); err == nil {
							w.Header().Set("Content-Type", versioned.MediaType)
						}
					}
				}
			}
		}
		fileHandler.ServeHTTP(w, req)
	}
	return http.HandlerFunc(handler)
}

func copyV2(source, destination string) error {
	err := filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
		relPath := strings.Replace(path, source, "", 1)
		if relPath == "" {
			return nil
		}
		switch m := info.Mode(); {
		case m&fs.ModeSymlink != 0: // Tag is the file name, so follow the symlink to the layer ID-named file.
			dst, err := os.Readlink(path)
			if err != nil {
				return err
			}
			id := filepath.Base(dst)
			if err := os.Symlink(id, filepath.Join(destination, relPath)); err != nil {
				return err
			}
		case m.IsDir():
			return os.Mkdir(filepath.Join(destination, relPath), 0755)
		default:
			data, err := ioutil.ReadFile(filepath.Join(source, relPath))
			if err != nil {
				return err
			}
			return ioutil.WriteFile(filepath.Join(destination, relPath), data, 0777)
		}
		return nil
	})
	return err
}
