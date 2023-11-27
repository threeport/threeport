package codegen

const ReconclierMarkerText = "threeport-codegen:reconciler"
const AllowDuplicateNamesMarkerText = "threeport-codegen:allow-duplicate-names"
const AddCustomMiddleware = "threeport-codegen:add-custom-middleware"
const DbLoadAssociations = "threeport-codegen:db-load-associations"

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
