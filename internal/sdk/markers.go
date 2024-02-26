package sdk

const ReconclierMarkerText = "threeport-sdk:reconciler"
const AllowDuplicateNamesMarkerText = "threeport-sdk:allow-duplicate-names"
const AddCustomMiddleware = "threeport-sdk:add-custom-middleware"
const DbLoadAssociations = "threeport-sdk:db-load-associations"

// These marker objects will be utilized if we add arguments to the marker.
// Leaving here in aniticipation of that.
//var (
//	ReconcilerMarkerDefinition = markers.Must(
//		markers.MakeDefinition(
//			ReconclierMarkerText,
//			markers.DescribesType,
//			ReconcilerMarker{},
//		),
//	)
//)
//
//type ReconcilerMarker struct{}
