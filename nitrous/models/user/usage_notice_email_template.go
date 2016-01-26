package user

import "text/template"

const usageNoticeEmailHtmlTmpl = `
<!DOCTYPE html>
<html>
  <head>
    <meta charset="utf-8">
    <title>Your free Nitrous workstation: {{ .HoursRemaining }} remaining this month</title>
  </head>
  <body>
    <h3>Hello {{ .Name }}!</h3>
    <p>You've used {{ .HoursUsed }} out of the free {{ .UsageQuotaInHour }} hours of usage of your Nitrous workstation. Your workstation will be unavailable after approximately another {{ .HoursRemaining }} of usage.</p>
    <p>You are allowed {{ .UsageQuotaInHour }} hours of usage every {{ .UsageQuotaPeriod }} days, so you will next be able to access your Nitrous workstation again on {{ .UsageQuotaNextRestoredAt }}.</p>
    <p>If you find yourself consistently exceeding the {{ .UsageQuotaInHour }}-hour limit, you should evaluate our paid plans - they grant unlimited usage of your workstation and it never suspends when you stop using it.</p>
    <br />
    <p>Have fun building things,</p>
    <p>The Nitrous team</p>
  </body>
</html>
`

const usageNoticeEmailTxtTmpl = `
Hello {{ .Name }}!

You've used {{ .HoursUsed }} out of the free {{ .UsageQuotaInHour }} hours of usage of your Nitrous workstation. Your workstation will be unavailable after approximately another {{ .HoursRemaining }} of usage.

You are allowed {{ .UsageQuotaInHour }} hours of usage every {{ .UsageQuotaPeriod }} days, so you will next be able to access your Nitrous workstation again on {{ .UsageQuotaNextRestoredAt }}.

If you find yourself consistently exceeding the {{ .UsageQuotaInHour }}-hour limit, you should evaluate our paid plans - they grant unlimited usage of your workstation and it never suspends when you stop using it.

Have fun building things,
The Nitrous team
`

var (
	usageNoticeEmailHtmlTemplate *template.Template
	usageNoticeEmailTxtTemplate  *template.Template
)

func init() {
	usageNoticeEmailHtmlTemplate =
		template.Must(template.New("usageNoticeEmailHtml").Parse(usageNoticeEmailHtmlTmpl))

	usageNoticeEmailTxtTemplate =
		template.Must(template.New("usageNoticeEmailTxt").Parse(usageNoticeEmailTxtTmpl))
}
