package event

// EventRecorder records events to the backend.
type EventRecorder struct {

	// The component reporting this event. Should be a short machine understandable string.
	Source string `validate:"optional"`

	// Name of the controller that emitted this Event, e.g. `kubernetes.io/kubelet`.
	ReportingController string `validate:"optional"`

	// ID of the controller instance, e.g. `kubelet-xyzf`.
	ReportingInstance string `validate:"optional"`
}

// Event records a new event with the given information.
func (r *EventRecorder) Event() {
	// if event exists, increment its count by 1
	// else, add new event

	// util.TypeName(v1.WorkloadInstance{})
	// event := &Event{
	// 	Reason:              "reason",
	// 	Message:             "message",
	// 	Source:              "source",
	// 	Count:               1,
	// 	Type:                "Normal",
	// 	EventTime:           time.Now(),
	// 	Action:              "action",
	// 	ReportingController: r.ReportingController,
	// 	ReportingInstance:   r.ReportingInstance,
	// }

	// AttachedObject: AttachedObjectReference{
	// 	ObjectType: ,
	// 	ObjectID: ,
	// 	AttachedObjectType: ,
	// 	AttachedObjectID: ,
	// },
}