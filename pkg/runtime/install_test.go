// Copyright 2022 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package runtime

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	gcp "github.com/GoogleCloudPlatform/buildpacks/pkg/gcpbuildpack"
	"github.com/GoogleCloudPlatform/buildpacks/pkg/testdata"
	"github.com/buildpacks/libcnb"
)

func TestInstallDartSDK(t *testing.T) {
	testCases := []struct {
		name         string
		httpStatus   int
		responseFile string
		wantFile     string
		wantError    bool
	}{
		{
			name:         "successful install",
			responseFile: "testdata/dummy-dart-sdk.zip",
			wantFile:     "lib/foo.txt",
		},
		{
			name:       "invalid version",
			httpStatus: http.StatusNotFound,
			wantError:  true,
		},
		{
			name:       "corrupt zip file",
			httpStatus: http.StatusOK,
			wantError:  true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := gcp.NewContext()
			l := &libcnb.Layer{
				Path:     t.TempDir(),
				Metadata: map[string]interface{}{},
			}

			stubFileServer(t, tc.httpStatus, tc.responseFile)

			version := "2.15.1"
			err := InstallDartSDK(ctx, l, version)

			if tc.wantError && err == nil {
				t.Fatalf("Expecting error but got nil")
			}
			if !tc.wantError && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tc.wantFile != "" {
				fp := filepath.Join(l.Path, tc.wantFile)
				if !ctx.FileExists(fp) {
					t.Errorf("Failed to extract. Missing file: %s", fp)
				}
				if l.Metadata["version"] != version {
					t.Errorf("Layer Metadata.version = %q, want %q", l.Metadata["version"], version)
				}
			}
		})
	}

}

func TestInstallRuby(t *testing.T) {
	testCases := []struct {
		name         string
		version      string
		httpStatus   int
		responseFile string
		wantFile     string
		wantVersion  string
		wantError    bool
	}{
		{
			name:         "successful install",
			version:      "2.x.x",
			responseFile: "testdata/dummy-ruby-runtime.tar.gz",
			wantFile:     "lib/foo.txt",
			wantVersion:  "2.2.2",
		},
		{
			name:         "default to highest available verions",
			responseFile: "testdata/dummy-ruby-runtime.tar.gz",
			wantFile:     "lib/foo.txt",
			wantVersion:  "3.3.3",
		},
		{
			name:         "invalid version",
			version:      ">9.9.9",
			responseFile: "testdata/dummy-ruby-runtime.tar.gz",
			wantError:    true,
		},
		{
			name:       "not found",
			version:    "2.2.2",
			httpStatus: http.StatusNotFound,
			wantError:  true,
		},
		{
			name:       "corrupt tar file",
			version:    "2.2.2",
			httpStatus: http.StatusOK,
			wantError:  true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			stubFileServer(t, tc.httpStatus, tc.responseFile)

			layer := &libcnb.Layer{
				Path:     t.TempDir(),
				Metadata: map[string]interface{}{},
			}
			ctx := gcp.NewContext()

			err := InstallTarball(ctx, Ruby, tc.version, layer)

			if tc.wantError == (err == nil) {
				t.Fatalf("InstallTarball(ctx, %q, %q) got error: %v, want error? %v", Ruby, tc.version, err, tc.wantError)
			}

			if tc.wantFile != "" {
				fp := filepath.Join(layer.Path, tc.wantFile)
				if !ctx.FileExists(fp) {
					t.Errorf("Failed to extract. Missing file: %s", fp)
				}
			}
			if tc.wantVersion != "" && layer.Metadata["version"] != tc.wantVersion {
				t.Errorf("Layer Metadata.version = %q, want %q", layer.Metadata["version"], tc.wantVersion)
			}
		})
	}
}

func stubFileServer(t *testing.T, httpStatus int, responseFile string) {
	t.Helper()
	svr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if httpStatus != 0 {
			w.WriteHeader(httpStatus)
		}
		if r.UserAgent() != gcpUserAgent {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if strings.Contains(r.URL.RawQuery, "getversions=1") {
			data, err := json.Marshal([]string{"1.1.1", "3.3.3", "2.2.2"})
			if err != nil {
				t.Fatalf("serializing versions: %v", err)
			}
			fmt.Fprint(w, string(data))
		} else if responseFile != "" {
			http.ServeFile(w, r, testdata.MustGetPath(responseFile))
		}
	}))
	t.Cleanup(svr.Close)

	origDartURL := dartSdkURL
	origTarballURL := googleTarballURL
	origVersionsURL := runtimeVersionsURL
	t.Cleanup(func() {
		dartSdkURL = origDartURL
		googleTarballURL = origTarballURL
		runtimeVersionsURL = origVersionsURL
	})
	dartSdkURL = svr.URL + "?version=%s"
	googleTarballURL = svr.URL + "?runtime=%s&version=%s"
	runtimeVersionsURL = svr.URL + "?runtime=%s&getversions=1"
}
