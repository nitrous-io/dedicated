package freehostusage

const (
	WarningSubject = `Workspace termination warning - 7 day notice`
	WarningMessage = `It looks like you have not been using your Nitrous workspace during the past month.

Your workspace {{ .Slug }} will be terminated in 7 days if you are not actively developing an application.

If you no longer need your workspace and its contents, no further action is required.

Should you wish to keep your workspace, simply start using it regularly.

Please contact us with any questions at support@nitrous.io .
`

	TerminatingSubject = `Workspace termination warning - final notice`
	TerminatingMessage = `Your workspace {{.Slug}} will be terminated in 3 days.

You can upgrade to a paid plan to keep it running.  If you do not wish to upgrade, please be sure to backup your files prior to termination.`

	TerminatedSubject = `Workspace termination notice`
	TerminatedMessage = `Your Nitrous Pro workspace has been terminated. You will no longer be able to access this workspace.`
)
