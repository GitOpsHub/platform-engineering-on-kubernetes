# Composition functions

Pipeline-mode Compositions do not contain patches or templates directly. Each pipeline step names a
**Function**, which runs as a gRPC service that Crossplane calls with the composite resource and the
desired state so far. A function returns the updated desired state.

| Function | Package tag | What this repo uses it for |
| -------- | ----------- | -------------------------- |
| `function-patch-and-transform` | `v0.8.2` | Renders managed resources from a base plus patches (`FromCompositeFieldPath`, `CombineFromComposite`) and transforms such as `map` and `string.Format`. Used by both compositions. |
| `function-auto-ready` | `v0.5.0` | Marks the composite Ready once every composed resource reports Ready. Last step in both pipelines. |

Why these two: `patch-and-transform` keeps the composition declarative and readable, with no template
language. `auto-ready` is needed because the default readiness rule would mark the composite ready
as soon as it is created, before the RDS instance exists.

Function packages are installed as `Function` objects (see `functions.yaml`). Crossplane pulls the image
and starts it as a Deployment. Pin the tag. Do not use `latest`.

To add a step, such as `function-go-templating` for logic that patches cannot express, add a new
`Function` object to `functions.yaml` and reference it by name from a pipeline step. Keep the
step order explicit: each step sees the output of the steps before it.
