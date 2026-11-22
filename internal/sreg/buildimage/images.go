// Copyright © 2021 Alibaba Group Holding Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package buildimage

import (
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/cpaas-dev/atoll-k8s/internal/utils/file"
	"github.com/cpaas-dev/atoll-k8s/internal/utils/logger"
	strutil "github.com/cpaas-dev/atoll-k8s/internal/utils/strings"
)

func formalizeImages(images []string) (res []string) {
	for i := range images {
		img := strings.TrimSpace(images[i])
		if img == "" || strings.HasPrefix(img, "#") {
			continue
		}
		res = append(res, strutil.TrimQuotes(img))
	}
	return
}

func ParseShimImages(dir string) ([]string, error) {
	if dir == "" || !file.IsExist(dir) {
		logger.Debug("path %s is not exists,skip", dir)
		return nil, nil
	}
	list := sets.NewString()
	paths, err := file.GetFiles(dir)
	if err != nil {
		return nil, fmt.Errorf("load image list files error: %w", err)
	}
	for _, p := range paths {
		images, err := file.ReadLines(p)
		if err != nil {
			return nil, fmt.Errorf("load image list error: %w", err)
		}
		list = list.Insert(images...)
	}
	imageList := formalizeImages(list.List())
	return imageList, nil
}
