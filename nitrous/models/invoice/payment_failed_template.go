package invoice

import "text/template"

// Updates to this template must be reflected in models/invoice_test

const paymentFailedHtmlTmpl = `
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
    <p>If we are unable to bill your credit card your workspaces will be <strong>terminated soon</strong>.</p>
    <p>If you have any questions please email our support team at <a href="mailto:{{ .SupportEmail }}">{{ .SupportEmail }}</a>.</p>
    <br />
    <p>Thanks,</p>
    <p>The Nitrous team</p>
  </body>
</html>
`

const paymentFailedTxtTmpl = `
Hello {{ .Name }},
Unfortunately, we have been unable to successfully bill your provided credit card ending in {{ .LastFour }} to the value of {{ .InvoiceAmount }}.
We will retry billing your card very shortly, but in the meantime please verify your credit card at {{ .BillingUrl }}.
If we are unable to bill your credit card your workspace will be **terminated soon**.
If you have any questions please email our support team at {{ .SupportEmail }}.

Thanks,
The Nitrous team
`

var (
	paymentFailedHtmlTemplate *template.Template
	paymentFailedTxtTemplate  *template.Template
)

func init() {
	paymentFailedHtmlTemplate =
		template.Must(template.New("paymentFailedHtml").Parse(paymentFailedHtmlTmpl))

	paymentFailedTxtTemplate =
		template.Must(template.New("paymentFailedTxt").Parse(paymentFailedTxtTmpl))
}
