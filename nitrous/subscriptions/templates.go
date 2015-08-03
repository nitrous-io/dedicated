package subscriptions

import "text/template"

const paymentFirstFailedHtmlTmpl = `
<!DOCTYPE html>
<html>
  <head>
    <meta charset="utf-8">
    <title>Nitrous - payment unsuccessful</title>
  </head>
  <body>
    <h3>Hello {{ .Name }},</h3>
    <p>Unfortunately, we have been unable to successfully bill your provided credit card ending in {{ .LastFour }} to the value of {{ .InvoiceAmount }}.</p>
    <p>We will retry billing your card very shortly, but in the meantime please <a href="{{ .BillingUrl }}">verify your credit card</a>.</p>
    <p>Your workspaces have been stopped and if we are unable to bill your credit card your workspaces will be <strong>terminated soon</strong>.</p>
    <p>If you have any questions please email our support team at <a href="mailto:{{ .SupportEmail }}">{{ .SupportEmail }}</a>.</p>
    <br />
    <p>Thanks,</p>
    <p>The Nitrous team</p>
  </body>
</html>
`

const paymentFirstFailedTxtTmpl = `
Hello {{ .Name }},
Unfortunately, we have been unable to successfully bill your provided credit card ending in {{ .LastFour }} to the value of {{ .InvoiceAmount }}.
We will retry billing your card very shortly, but in the meantime please verify your credit card at {{ .BillingUrl }}.
Your workspaces have been stopped and if we are unable to bill your credit card your workspace will be **terminated soon**.
If you have any questions please email our support team at {{ .SupportEmail }}.

Thanks,
The Nitrous team
`

const paymentSecondFailedHtmlTmpl = `
<!DOCTYPE html>
<html>
  <head>
    <meta charset="utf-8">
    <title>Nitrous - payment retry unsuccessful</title>
  </head>
  <body>
    <h3>Hello {{ .Name }},</h3>
    <p>We are still having issues successfully billing your provided credit card ending in {{ .LastFour }} to the value of {{ .InvoiceAmount }}.</p>
    <p>We will retry billing your card in 3 days, but in the meantime please <a href="{{ .BillingUrl }}">verify your credit card</a>.</p>
    <p>Your workspaces have been stopped and if we are unable to bill your credit card by the 10th your workspaces will be <strong>terminated soon</strong>.</p>
    <p>If you have any questions please email our support team at <a href="mailto:{{ .SupportEmail }}">{{ .SupportEmail }}</a>.</p>
    <br />
    <p>Thanks,</p>
    <p>The Nitrous team</p>
  </body>
</html>
`

const paymentSecondFailedTxtTmpl = `
Hello {{ .Name }},
We are still having issues successfully billing your provided credit card ending in {{ .LastFour }} to the value of {{ .InvoiceAmount }}.
We will retry billing your card in 3 days, but in the meantime please verify your credit card at {{ .BillingUrl }}.
Your workspaces have been stopped and if we are unable to bill your credit card by the 10th your workspace will be **terminated soon**.
If you have any questions please email our support team at {{ .SupportEmail }}.

Thanks,
The Nitrous team
`

const paymentThirdFailedHtmlTmpl = `
<!DOCTYPE html>
<html>
  <head>
    <meta charset="utf-8">
    <title>Nitrous - final warning</title>
  </head>
  <body>
    <h3>Hello {{ .Name }},</h3>
    <p>We are still having issues successfully billing your provided credit card ending in {{ .LastFour }} to the value of {{ .InvoiceAmount }}.</p>
    <p>We will retry billing your card very shortly, but in the meantime please <a href="{{ .BillingUrl }}">verify your credit card</a>.</p>
    <p>If we are unable to bill your credit card by the 10th your workspaces will be <strong>terminated after our next retry</strong>.</p>
    <p>If you have any questions please email our support team at <a href="mailto:{{ .SupportEmail }}">{{ .SupportEmail }}</a>.</p>
    <br />
    <p>Thanks,</p>
    <p>The Nitrous team</p>
  </body>
</html>
`

const paymentThirdFailedTxtTmpl = `
Hello {{ .Name }},
We are still having issues successfully billing your provided credit card ending in {{ .LastFour }} to the value of {{ .InvoiceAmount }}.
We will retry billing your card very shortly, but in the meantime please verify your credit card at {{ .BillingUrl }}.
Your workspaces have been stopped and if we are unable to bill your credit card by the 10th your workspace will be **terminated after our next retry**.
If you have any questions please email our support team at {{ .SupportEmail }}.

Thanks,
The Nitrous team
`

const paymentFinalFailedHtmlTmpl = `
<!DOCTYPE html>
<html>
  <head>
    <meta charset="utf-8">
    <title>Nitrous - your workspaces will be terminated</title>
  </head>
  <body>
    <h3>Hello {{ .Name }},</h3>
    <p>Unfortunately, we have been unable to successfully bill your provided credit card ending in {{ .LastFour }} to the value of {{ .InvoiceAmount }}.</p>
    <p>Your workspaces have been marked for termination and will be terminated soon, to avoid this termination please <a href="{{ .BillingUrl }}">verify your credit card</a>.</p>
    <p>If you have any questions please email our support team at <a href="mailto:{{ .SupportEmail }}">{{ .SupportEmail }}</a>.</p>
    <br />
    <p>Thanks,</p>
    <p>The Nitrous team</p>
  </body>
</html>
`

const paymentFinalFailedTxtTmpl = `
Hello {{ .Name }},
Unfortunately, we have been unable to successfully bill your provided credit card ending in {{ .LastFour }} to the value of {{ .InvoiceAmount }}.
Your workspaces have been marked for termination and will be terminated soon, to avoid this termination please verify your credit card at {{ .BillingUrl }}.
If you have any questions please email our support team at {{ .SupportEmail }}.

Thanks,
The Nitrous team
`

var (
	paymentFirstFailedHtmlTemplate *template.Template
	paymentFirstFailedTxtTemplate  *template.Template

	paymentSecondFailedHtmlTemplate *template.Template
	paymentSecondFailedTxtTemplate  *template.Template

	paymentThirdFailedHtmlTemplate *template.Template
	paymentThirdFailedTxtTemplate  *template.Template

	paymentFinalFailedHtmlTemplate *template.Template
	paymentFinalFailedTxtTemplate  *template.Template
)

func init() {
	paymentFirstFailedHtmlTemplate =
		template.Must(template.New("paymentFirstFailedHtml").Parse(paymentFirstFailedHtmlTmpl))
	paymentFirstFailedTxtTemplate =
		template.Must(template.New("paymentFirstFailedTxt").Parse(paymentFirstFailedTxtTmpl))

	paymentSecondFailedHtmlTemplate =
		template.Must(template.New("paymentSecondFailedHtml").Parse(paymentSecondFailedHtmlTmpl))
	paymentSecondFailedTxtTemplate =
		template.Must(template.New("paymentSecondFailedTxt").Parse(paymentSecondFailedTxtTmpl))

	paymentThirdFailedHtmlTemplate =
		template.Must(template.New("paymentThirdFailedHtml").Parse(paymentThirdFailedHtmlTmpl))
	paymentThirdFailedTxtTemplate =
		template.Must(template.New("paymentThirdFailedTxt").Parse(paymentThirdFailedTxtTmpl))

	paymentFinalFailedHtmlTemplate =
		template.Must(template.New("paymentFinalFailedHtml").Parse(paymentFinalFailedHtmlTmpl))

	paymentFinalFailedTxtTemplate =
		template.Must(template.New("paymentFinalFailedTxt").Parse(paymentFinalFailedTxtTmpl))
}
