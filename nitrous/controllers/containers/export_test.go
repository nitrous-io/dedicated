package containers

var (
	PrepareConfig     = prepareConfig
	ErrVolumeNotFound = errVolumeNotFound
	ErrImageNotFound  = errImageNotFound
	ErrMissingCmd     = errMissingCmd
	ErrNameRequired   = errNameRequired
	Validations       = validations

	DoExclusive      = doExclusive
	DoAsyncExclusive = doAsyncExclusive
	LockerFn         = lockerFn
	UnlockerFn       = unlockerFn

	FetchContainersWithHosts = fetchContainersWithHosts
)
