package idlehosts

import "text/template"

const firstWarningHtmlTmpl = `
<!DOCTYPE html>
<html>
  <head>
    <meta charset="utf-8">
    <title>Nitrous - your workspace '{{ .HostName }}' will be deleted in 1 week</title>
  </head>
  <body>
    <h3>Hi {{ .Name }},</h3>
    <p>We noticed you haven't used your free workspace <strong>{{ .HostName }}</strong> in the past week. In an effort to maintain the best possible performance for all Nitrous users, we terminate unused workspaces. If you do not use your workspace in the next 7 days, it will be deleted.</p>
    <p>To save your free projects and begin using your workspace again, visit the <a href="{{ .DashboardUrl }}">visit the dashboard</a> and start your workspace. You can also avoid inactive workspace termination in the future by <a href="{{ .UpgradeUrl }}">upgrading to any of our paid plans</a>.</p>
    <p>Please note that we are not deleting your user account, only the free workspaces you are not using. You can sign in again anytime to create a new free workspace.</p>
    <p>If you have any questions please email our support team at {{ .SupportEmail }}.</p>
    <p>Thanks,</p>
    <p>The Nitrous team</p>
  </body>
</html>
`

const firstWarningTxtTmpl = `
Hi {{ .Name }},

We noticed you haven't used your free workspace {{ .HostName }} in the past week. In an effort to maintain the best possible performance for all Nitrous users, we terminate unused workspaces. If you do not use your workspace in the next 7 days, it will be deleted.

To save your free projects and begin using your workspace again, visit the dashboard at {{ .DashboardUrl }} and start your workspace. You can also avoid inactive workspace termination in the future by upgrading to any of our paid plans at {{ .UpgradeUrl }}.

Please note that we are not deleting your user account, only the free workspaces you are not using. You can sign in again anytime to create a new free workspace.

If you have any questions please email our support team at {{ .SupportEmail }}.

Thanks,
The Nitrous Team
`

const finalWarningHtmlTmpl = `
<!DOCTYPE html>
<html>
  <head>
    <meta charset="utf-8">
    <title>Nitrous - your workspace '{{ .HostName }}' will be deleted in 24 hours</title>
  </head>
  <body>
    <h3>Hi {{ .Name }},</h3>
    <p>We noticed you haven't used your free workspace <strong>{{ .HostName }}</strong> in the past two weeks. In an effort to maintain the best possible performance for all Nitrous users, we terminate unused workspaces. If you do not use your workspace in the next 24 hours, it will be deleted.</p>
    <p>To save your free projects and begin using your workspace again, <a href="{{ .DashboardUrl }}">visit the dashboard</a> and start your workspace. You can also avoid inactive workspace termination in the future by <a href="{{ .UpgradeUrl }}">upgrading to any of our paid plans</a>.</p>
    <p>Please note that we are not deleting your user account, only the free workspaces you are not using. You can sign in again anytime to create a new free workspace.</p>
    <p>If you have any questions please email our support team at {{ .SupportEmail }}.</p>
    <p>Thanks,</p>
    <p>The Nitrous team</p>
  </body>
</html>
`

const finalWarningTxtTmpl = `
Hi {{ .Name }},

We noticed you haven't used your free workspace {{ .HostName }} in the past two weeks. In an effort to maintain the best possible performance for all Nitrous users, we terminate unused workspaces. If you do not use your workspace in the next 24 hours, it will be deleted.

To save your free projects and begin using your workspace again, visit the dashboard at {{ .DashboardUrl }} and start your workspace. You can also avoid inactive workspace termination in the future by upgrading to any of our paid plans at {{ .UpgradeUrl }}.

Please note that we are not deleting your user account, only the free workspaces you are not using. You can sign in again anytime to create a new free workspace.

If you have any questions please email our support team at {{ .SupportEmail }}.

Thanks,
The Nitrous Team
`

var (
	firstWarningHtmlTemplate *template.Template
	firstWarningTxtTemplate  *template.Template

	finalWarningHtmlTemplate *template.Template
	finalWarningTxtTemplate  *template.Template
)

func init() {
	firstWarningHtmlTemplate =
		template.Must(template.New("firstWarningHtml").Parse(firstWarningHtmlTmpl))
	firstWarningTxtTemplate =
		template.Must(template.New("firstWarningTxt").Parse(firstWarningTxtTmpl))

	finalWarningHtmlTemplate =
		template.Must(template.New("finalWarningHtml").Parse(finalWarningHtmlTmpl))
	finalWarningTxtTemplate =
		template.Must(template.New("finalWarningTxt").Parse(finalWarningTxtTmpl))

}
