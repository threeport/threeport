package v0

// ApiObjectVersions provides the versions for individual REST endpoints.
type ApiObjectVersions struct {
	//  Required.  REST API resource name.
	API string `json:"API" validate:"required"`
	// Required.  REST API resource versions.
	Versions []string `json:"Versions" validate:"required"`
}

var ObjectVersions = make(map[string]ApiObjectVersions)

// AddObjectVersion addes the provided object version to the ObjectVersions map.
func AddObjectVersion(vo VersionObject) {
	var Exists bool = false
	for _, v := range ObjectVersions {
		if v.API == vo.Object {
			Exists = true
		}
	}
	if !Exists {
		apiVer := new(ApiObjectVersions)
		apiVer.API = vo.Object
		apiVer.Versions = []string{vo.Version}
		ObjectVersions[vo.Object] = *apiVer
	} else {
		for _, v := range ObjectVersions[vo.Object].Versions {
			if v == vo.Version {
				return
			}
		}
		x := ObjectVersions[vo.Object]
		x.Versions = append(x.Versions, vo.Version)
		ObjectVersions[vo.Object] = x
	}
}
