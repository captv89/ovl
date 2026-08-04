# proto

Protobuf definitions for the vessel-office sync contract (ConnectRPC), per
architecture handoff section 11. Authoring the contract is Phase 1 work;
wiring actual ConnectRPC services on both `vessel/` and `office/` against it
is Phase 4 (architecture handoff section 19).

`ovl/sync/v1/sync.proto` defines `SyncService` (`PushOutbox`,
`QueryMissingAttachmentChunks`, `UploadAttachmentChunk`, `PullInbox`,
`SyncStatus`) and its message types.

## Regenerating

Requires `buf`, `protoc-gen-go`, and `protoc-gen-connect-go` on `PATH`
(`go install` each from their respective module paths). Then, from the repo
root:

```
make proto
```

This lints `proto/` and regenerates `pkg/syncproto/gen/` from
`buf.gen.yaml`. Never hand-edit files under `pkg/syncproto/gen/`.
