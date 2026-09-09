package canvas

import "fmt"

// Action values are the server-side MCP tool names the Pinner views can
// trigger through the MCP Apps bridge (callServerTool). The vocabulary is
// derived from the reference screens' entrypoint configs (pinner-cli
// packages/apps/src/entries/*.ts) and doubles as the tool-name contract the
// server packages must keep stable: a view whose action no longer resolves
// to a registered tool is a broken screen.
//
// Actions are a versioned compatibility surface (see ContractVersion);
// removing or renaming one is a breaking change.
type Action string

const (
	// ActionPinsAdd submits a new pin. Triggered by Pin.
	ActionPinsAdd Action = "pins_add"
	// ActionPinStatus polls a submitted pin's status. Triggered by Pin.
	ActionPinStatus Action = "pin_status"
	// ActionPinsList reads the account's pins. Triggered by PinList.
	ActionPinsList Action = "pins_list"
	// ActionVaultStatus reads vault open/reachability state. Triggered by
	// VaultBrowser.
	ActionVaultStatus Action = "vault_status"
	// ActionVaultLS lists a vault path. Triggered by VaultBrowser.
	ActionVaultLS Action = "vault_ls"
	// ActionVaultCreate starts vault creation. Triggered by VaultCreate.
	ActionVaultCreate Action = "vault_create"
	// ActionVaultCreateStatus polls vault creation. Triggered by VaultCreate.
	ActionVaultCreateStatus Action = "vault_create_status"
	// ActionVaultRestore starts a vault restore. Triggered by VaultRestore.
	ActionVaultRestore Action = "vault_restore"
	// ActionVaultRestoreStatus polls a vault restore. Triggered by
	// VaultRestore.
	ActionVaultRestoreStatus Action = "vault_restore_status"
	// ActionVaultUploadSubmit uploads a file to the vault. Triggered by
	// VaultUpload.
	ActionVaultUploadSubmit Action = "vault_upload_submit"
	// ActionVaultGetFile downloads a vault file. Triggered by VaultDownload.
	ActionVaultGetFile Action = "vault_get_file"
	// ActionIPFSUploadSubmit starts an IPFS upload. Triggered by IPFSUpload.
	ActionIPFSUploadSubmit Action = "ipfs_upload_submit"
	// ActionIPFSUploadStatus polls an IPFS upload. Triggered by IPFSUpload.
	ActionIPFSUploadStatus Action = "ipfs_upload_status"
	// ActionDownloadFile downloads an IPFS CID. Triggered by IPFSDownload.
	ActionDownloadFile Action = "download_file"
	// ActionAuthSSO starts the SSO sign-in flow. Triggered by AuthSSO.
	ActionAuthSSO Action = "auth_sso"
	// ActionAuthSSOStatus polls the SSO sign-in flow. Triggered by AuthSSO.
	ActionAuthSSOStatus Action = "auth_sso_status"
	// ActionAuthSSORevoke revokes the SSO session. Triggered by AuthSSO.
	ActionAuthSSORevoke Action = "auth_sso_revoke"
	// ActionAuthStatus reads the current account/auth status. Triggered by
	// AuthStatus.
	ActionAuthStatus Action = "auth_status"
	// ActionAccountPasswordUpdate starts the password change. Triggered by
	// AccountPassword.
	ActionAccountPasswordUpdate Action = "account_password_update"
	// ActionAccountEmailChange starts the email change. Triggered by
	// AccountEmail.
	ActionAccountEmailChange Action = "account_email_change"
)

// viewActions is the authoritative view→action table for ContractVersion 1,
// derived one-to-one from the reference screens' entrypoint tool configs
// (pinner-cli packages/apps/src/entries/*.ts). Mutating actions appear only
// on the views that can invoke them.
var viewActions = map[View][]Action{
	ViewPin:             {ActionPinsAdd, ActionPinStatus},
	ViewPinList:         {ActionPinsList},
	ViewVaultBrowser:    {ActionVaultStatus, ActionVaultLS},
	ViewVaultCreate:     {ActionVaultCreate, ActionVaultCreateStatus},
	ViewVaultRestore:    {ActionVaultRestore, ActionVaultRestoreStatus},
	ViewVaultUpload:     {ActionVaultUploadSubmit},
	ViewVaultDownload:   {ActionVaultGetFile},
	ViewIPFSUpload:      {ActionIPFSUploadSubmit, ActionIPFSUploadStatus},
	ViewIPFSDownload:    {ActionDownloadFile},
	ViewAuthSSO:         {ActionAuthSSO, ActionAuthSSOStatus, ActionAuthSSORevoke},
	ViewAuthStatus:      {ActionAuthStatus},
	ViewAccountPassword: {ActionAccountPasswordUpdate},
	ViewAccountEmail:    {ActionAccountEmailChange},
}

// Actions returns the tool invocations v can trigger, in contract order
// (the order the reference screens acquire them). It returns an error
// wrapping ErrUnknownView for an unregistered view. Send/submit actions
// come first, then status/poll actions, then revocations.
func (v View) Actions() ([]Action, error) {
	if !v.IsValid() {
		return nil, fmt.Errorf("%w: %q", ErrUnknownView, string(v))
	}
	actions := make([]Action, len(viewActions[v]))
	copy(actions, viewActions[v])
	return actions, nil
}

// State is one client-side UI state in a view's state machine, rendered as
// the view's status line. The vocabulary is the union of the reference
// screens' machine states (pinner-cli packages/apps/src/*.ts); each view's
// allowed subset is fixed by States.
//
// States are a versioned compatibility surface (see ContractVersion);
// removing or renaming one is a breaking change.
type State string

const (
	// StateIdle is the quiescent form state before a submit.
	StateIdle State = "idle"
	// StateForm is an active, unsubmitted form.
	StateForm State = "form"
	// StateFormError is a form rejected for bad input.
	StateFormError State = "form_error"
	// StateStarting is a submitted flow awaiting its first result.
	StateStarting State = "starting"
	// StateMinting is an upload reserving its destination storage.
	StateMinting State = "minting"
	// StateUploading is an upload transferring bytes.
	StateUploading State = "uploading"
	// StateDownloading is a download transferring bytes.
	StateDownloading State = "downloading"
	// StateSubmitting is a pin form submit in flight.
	StateSubmitting State = "submitting"
	// StatePolling is a flow awaiting a terminal status read.
	StatePolling State = "polling"
	// StateLoading is a read view fetching its data.
	StateLoading State = "loading"
	// StateReady is a read view showing loaded data.
	StateReady State = "ready"
	// StateInfo is a pin flow showing non-terminal informational state.
	StateInfo State = "info"
	// StateOk is a flow or transfer completed successfully.
	StateOk State = "ok"
	// StateDead is a flow terminated unrecoverably server-side.
	StateDead State = "dead"
	// StateNoURL is a link flow whose start returned no usable URL.
	StateNoURL State = "nourl"
	// StateError is a flow, read, or transfer that failed.
	StateError State = "error"
	// StateTimeout is a flow that exhausted its status-polling budget.
	StateTimeout State = "timeout"
	// StateRevoking is an SSO flow revoking the session.
	StateRevoking State = "revoking"
)

// viewStates is the authoritative view→state table for ContractVersion 1,
// derived from the reference screens' state machines
// (packages/apps/src: PinState, PinListState, BrowserState, IPFSUploadState,
// VaultUploadState, FlowState, LinkState, AuthStatusState, DownloadState).
var viewStates = map[View][]State{
	ViewPin:             {StateForm, StateFormError, StateSubmitting, StatePolling, StateOk, StateInfo, StateError, StateTimeout},
	ViewPinList:         {StateLoading, StateReady, StateError},
	ViewVaultBrowser:    {StateLoading, StateReady, StateError},
	ViewVaultCreate:     {StateIdle, StateStarting, StatePolling, StateOk, StateDead, StateError, StateTimeout},
	ViewVaultRestore:    {StateIdle, StateStarting, StatePolling, StateOk, StateDead, StateError, StateTimeout},
	ViewVaultUpload:     {StateForm, StateMinting, StateUploading, StateOk, StateError},
	ViewVaultDownload:   {StateIdle, StateDownloading, StateOk, StateError},
	ViewIPFSUpload:      {StateIdle, StateMinting, StateUploading, StatePolling, StateOk, StateError},
	ViewIPFSDownload:    {StateIdle, StateDownloading, StateOk, StateError},
	ViewAuthSSO:         {StateIdle, StateStarting, StatePolling, StateOk, StateDead, StateError, StateTimeout, StateRevoking},
	ViewAuthStatus:      {StateLoading, StateReady, StateError},
	ViewAccountPassword: {StateIdle, StateStarting, StateOk, StateNoURL, StateError},
	ViewAccountEmail:    {StateIdle, StateStarting, StateOk, StateNoURL, StateError},
}

// States returns the UI state vocabulary v may occupy, in lifecycle order.
// It returns an error wrapping ErrUnknownView for an unregistered view.
func (v View) States() ([]State, error) {
	if !v.IsValid() {
		return nil, fmt.Errorf("%w: %q", ErrUnknownView, string(v))
	}
	states := make([]State, len(viewStates[v]))
	copy(states, viewStates[v])
	return states, nil
}

// PinStatusPayload is the pin status readout ViewPin renders while polling
// ActionPinStatus. It mirrors pinner-cli internal/mcp/apps PinStatusView
// and the status member the pin screen reads natively from pin_status.
type PinStatusPayload struct {
	// CID is the pinned identifier submitted to the pin create flow.
	CID string `json:"cid"`
	// Status is the current pin state (e.g. "pending", "pinned", "failed").
	Status string `json:"status"`
}

// PinRow is one entry of the pins listing the ViewPinList table renders. It
// mirrors the PinRow shape the pin-list screen reads from ActionPinsList
// results (packages/apps/src/pin-list.ts). Fields other than CID are
// optional server data and may be empty.
type PinRow struct {
	// CID is the pin's content identifier.
	CID string `json:"cid"`
	// Name is the optional human-assigned pin name.
	Name string `json:"name,omitempty"`
	// Status is the pin's current state.
	Status string `json:"status,omitempty"`
	// Created is the pin's creation timestamp.
	Created string `json:"created,omitempty"`
	// RequestID is the optional pinning request identifier.
	RequestID string `json:"request_id,omitempty"`
	// Metadata is the optional metadata map stored with the pin.
	Metadata map[string]string `json:"metadata,omitempty"`
}

// PinListPayload is the listing ViewPinList renders in StateReady. It is a
// top-level array of rows, not an object: the pin-list client reads the
// envelope's value as a bare PinRow[] (Array.isArray in
// packages/apps/src/pin-list.ts), mirroring how AuthStatusPayload and
// VaultStatusPayload mirror their object-shaped envelope values. An empty
// (nil) payload means no pins yet — an empty readout, not an error.
type PinListPayload []PinRow

// AuthStatusPayload is the account readout ViewAuthStatus renders in
// StateReady. It mirrors the AuthStatusData shape the auth-status screen
// reads from ActionAuthStatus results (packages/apps/src/auth-status.ts).
type AuthStatusPayload struct {
	// Authenticated reports whether the session is signed in.
	Authenticated bool `json:"authenticated,omitempty"`
	// PortalURL is the portal sign-in URL to surface when not
	// authenticated.
	PortalURL string `json:"portal_url,omitempty"`
	// Message is the human-readable account status message.
	Message string `json:"message,omitempty"`
}

// VaultStatusPayload is the vault readout ViewVaultBrowser renders in
// StateReady. It mirrors the VaultStatus shape the browser reads from
// ActionVaultStatus results (packages/apps/src/vault-browser.ts). Boolean
// and numeric fields are absent (zero) when the vault is closed, matching
// the optional-member JS shape.
type VaultStatusPayload struct {
	// Unlocked reports whether the vault is open locally.
	Unlocked bool `json:"unlocked,omitempty"`
	// RemoteReachable reports whether the remote vault backend answers.
	RemoteReachable bool `json:"remote_reachable,omitempty"`
	// RemoteReady reports whether the remote vault is ready for I/O.
	RemoteReady bool `json:"remote_ready,omitempty"`
	// RemoteError is the last remote error detail, when not ready.
	RemoteError string `json:"remote_error,omitempty"`
	// StorageUsed is the vault's used storage in bytes.
	StorageUsed int64 `json:"storage_used,omitempty"`
	// StorageLimit is the vault's storage quota in bytes (0 = unlimited).
	StorageLimit int64 `json:"storage_limit,omitempty"`
	// RemainingStorage is the vault's free storage in bytes.
	RemainingStorage int64 `json:"remaining_storage,omitempty"`
	// CacheState is the local vault cache state ("missing" or "healthy").
	CacheState string `json:"cache_state,omitempty"`
}

// Payload returns the zero value of v's typed server-provided payload
// contract — the initial data a server hands into the view to seed it. It
// returns an error wrapping ErrUnknownView for an unregistered view and
// ErrMissingPayload for the views the server passes no init payload into:
// those screens bootstrap themselves through their Actions instead, and no
// payload type is padded in for them.
func (v View) Payload() (any, error) {
	if !v.IsValid() {
		return nil, fmt.Errorf("%w: %q", ErrUnknownView, string(v))
	}
	switch v {
	case ViewPin:
		return PinStatusPayload{}, nil
	case ViewPinList:
		return PinListPayload{}, nil
	case ViewAuthStatus:
		return AuthStatusPayload{}, nil
	case ViewVaultBrowser:
		return VaultStatusPayload{}, nil
	default:
		// Flow, upload, download, and link views (VaultCreate, VaultRestore,
		// VaultUpload, VaultDownload, IPFSUpload, IPFSDownload, AuthSSO,
		// AccountPassword, AccountEmail) receive no server-provided payload:
		// their forms start empty and their results arrive through action
		// tool results, so the contract deliberately declares no shape.
		return nil, fmt.Errorf("%w: %q", ErrMissingPayload, string(v))
	}
}
