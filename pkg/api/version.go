package api

import (
	_ "embed"

	iapi "github.com/threeport/threeport/internal/api"
)

// RESTAPIVersions provides the versions for individual REST endpoints.
type RESTAPIVersions struct {
	//  Required.  REST API resource name.
	API string `json:"API" validate:"required"`
	// Required.  REST API resource versions.
	Versions []string `json:"Versions" validate:"required"`
}

var RestapiVersions = make(map[string]RESTAPIVersions)

func AddRestApiVersion(vo iapi.VersionObject) {
	var Exists bool = false
	for _, v := range RestapiVersions {
		if v.API == vo.Object {
			Exists = true
		}
	}
	if !Exists {
		apiVer := new(RESTAPIVersions)
		apiVer.API = vo.Object
		apiVer.Versions = []string{vo.Version}
		RestapiVersions[vo.Object] = *apiVer
	} else {
		for _, v := range RestapiVersions[vo.Object].Versions {
			if v == vo.Version {
				return
			}
		}
		x := RestapiVersions[vo.Object]
		x.Versions = append(x.Versions, vo.Version)
		RestapiVersions[vo.Object] = x
	}
}
