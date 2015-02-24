package invoice

import "text/template"

// Updates to this template must be reflected in models/invoice_test

const finalPaymentFailedHtmlTmpl = `
<!DOCTYPE html>
<html>
  <head>
    <meta charset="utf-8">
    <title>Nitrous - final payment unsuccessful</title>
  </head>
  <body>
    <h3>Hello {{ .Name }},</h3>
    <p>Unfortunately, we have been unable to successfully bill your provided credit card ending in {{ .LastFour }} to the value of {{ .InvoiceAmount }}.</p>
    <p>Your workspaces <strong>will be terminated</strong> very soon, please <a href="{{ .BillingUrl }}">verify your credit card</a> to restore service.</p>
    <p>If you have any questions please email our support team at <a href="mailto:{{ .SupportEmail }}">{{ .SupportEmail }}</a>.</p>
    <br />
    <p>Thanks,</p>
    <p>The Nitrous team</p>
  </body>
</html>
`

const finalPaymentFailedTxtTmpl = `
Hello {{ .Name }},
Unfortunately, we have been unable to successfully bill your provided credit card ending in {{ .LastFour }} to the value of {{ .InvoiceAmount }}.
Your workspaces **will be terminated** very soon, please verify your credit card at {{ .BillingUrl }} to restore service.
If you have any questions please email our support team at {{ .SupportEmail }}.

Thanks,
The Nitrous team
`

var (
	finalPaymentFailedHtmlTemplate *template.Template
	finalPaymentFailedTxtTemplate  *template.Template
)

func init() {
	finalPaymentFailedHtmlTemplate =
		template.Must(template.New("finalPaymentFailedHtml").Parse(finalPaymentFailedHtmlTmpl))

	finalPaymentFailedTxtTemplate =
		template.Must(template.New("finalPaymentFailedTxt").Parse(finalPaymentFailedTxtTmpl))
}
