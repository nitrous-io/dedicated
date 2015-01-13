#= require mailcheck

App.SignupView = Em.View.extend
  elementId: 'signup-view'

  codeFieldInvalid:     Em.computed.equal('controller.invalidField', 'code')
  usernameFieldInvalid: Em.computed.equal('controller.invalidField', 'username')
  emailFieldInvalid:    Em.computed.equal('controller.invalidField', 'email')
  passwordFieldInvalid: Em.computed.equal('controller.invalidField', 'password')

  disableSubmitButton: (->
    !@get('controller.user.username') || !@get('controller.user.email') || !@get('controller.user.password') || @get('controller.signingUp')
  ).property 'controller.user.username', 'controller.user.email', 'controller.user.password', 'controller.signingUp'

  focusInvalidField: (->
    if field = @get 'controller.invalidField'
      @$("#signup-#{field}").focus()
  ).observes('controller.invalidField')

  checkEmail: (->
    Mailcheck.run(
      domains: ["gmail.com", "googlemail.com", "yahoo.com", "yahoomail.com", "ymail.com", "yahoo.ca", "yahoo.cn", "yahoo.co.id", "yahoo.co.in", "yahoo.co.jp", "yahoo.co.kr", "yahoo.co.nz", "yahoo.co.uk", "yahoo.de", "yahoo.dk", "yahoo.es", "yahoo.fr", "yahoo.gr", "yahoo.ie", "yahoo.in", "yahoo.it", "yahoo.pl", "yahoo.ro", "yahoo.ru", "yahoo.se", "yahoo.com.ar", "yahoo.com.au", "yahoo.com.br", "yahoo.com.cn", "yahoo.com.hk", "yahoo.com.mx", "yahoo.com.my", "yahoo.com.ph", "yahoo.com.sg", "yahoo.com.tw", "yahoo.com.vn", "hotmail.com", "live.com", "hotmail.co.uk", "hotmail.co.jp", "msn.com", "me.com", "mac.com", "icloud.com", "naver.com", "hanmail.net", "daum.net", "aol.com", "comcast.net", "facebook.com", "verizon.net", "gmx.com", "gmx.net", "gmx.de", "mail.com", "fastmail.fm", "yandex.com", "yandex.com.tr", "yandex.by", "yandex.ru", "yandex.ua"]
      topLevelDomains: ["co.jp", "co.uk", "com", "net", "org", "info", "edu", "gov", "mil", "ru", "de", "dk", "es", "fr", "gr", "ie", "in", "pl", "ro", "se", "ca", "cn", "co.id", "co.in", "co.kr", "co.nz", "fm", "io", "ua", "com.tr"]

      email: @get('controller.user.email')

      suggested: (s) =>
        @set('controller.suggestedEmail', s.full)

      empty: (s) =>
        @set('controller.suggestedEmail', null)
    )
  ).observes('controller.user.email')
