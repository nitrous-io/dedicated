package invoice

import "text/template"

const finalPaymentFailedHtmlTmpl = `
<!DOCTYPE html>
<html>
  <head>
    <meta charset="utf-8">
    <title>Nitrous - payment unsuccessful</title>
  </head>
  <body>
    <h3>Hello {{ .Name }},</h3>
    <p>Unfortunately, we have been unable to successfully bill your provided credit card ending in {{ .LastFour }} to the value of {{ .InvoiceAmount }}.</p>
    <p>To rectify this issue please contact our support team at <a href="mailto:{{ .SupportEmail }}">{{ .SupportEmail }}</a>.</p>
    <br />
    <p>Thanks,</p>
    <p>The Nitrous team</p>
  </body>
</html>
`

const finalPaymentFailedTxtTmpl = `
Hello {{ .Name }},</h3>
Unfortunately, we have been unable to successfully bill your provided credit card ending in {{ .LastFour }} to the value of {{ .InvoiceAmount }}.
To rectify this issue please contact our support team at {{ .SupportEmail }}.</p>

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
