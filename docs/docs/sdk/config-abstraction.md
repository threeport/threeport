# Config Abstraction

> Guide using helm and terraform to deploy app


## Replace

`threeport-sdk gen` scaffolds a `Replace` method on each object's config in
`pkg/config/v0`, for `tptctl replace`. It sends the whole object to the API
with a PUT, and a PUT writes every field, a nil one included.

The scaffold starts the object it sends as a copy of the one it fetched:

```go
// construct updated widget instance object
updatedWidgetInstance := util.Ptr(*existingWidgetInstance)
updatedWidgetInstance.Name = widgetInstanceValues.Name
// TODO: add API object fields as needed for WidgetInstance
```

So any field you don't assign keeps the value the API returned. That covers
the fields a controller owns, such as `Reconciled`, the rest of
`Reconciliation`, and `Status`. Don't assign those yourself; the copy already
carries them.

For each field a user sets through the config, assign the config's value
unconditionally, even when it is nil:

```go
// right: leaving the field out of the config clears it
updatedWidgetInstance.Replicas = widgetInstanceValues.Replicas

// wrong: leaving the field out of the config keeps the old value forever
if widgetInstanceValues.Replicas != nil {
	updatedWidgetInstance.Replicas = widgetInstanceValues.Replicas
}
```

The config a user replaces with is the full declared state of the fields they
manage. Leaving an optional field out is how they clear it, and a conditional
assignment silently keeps the old value instead.

A field the config refers to by name, such as a definition or a provider,
resolves to an ID the same way `Create` resolves it. When the config names
nothing, assign nil.
