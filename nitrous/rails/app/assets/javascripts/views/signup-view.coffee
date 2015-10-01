#= require mailcheck

`
var domains = [
  /* Default domains included */
  "aol.com", "att.net", "comcast.net", "facebook.com", "gmail.com", "gmx.com", "googlemail.com",
  "google.com", "hotmail.com", "hotmail.co.uk", "mac.com", "me.com", "mail.com", "msn.com",
  "live.com", "sbcglobal.net", "verizon.net", "yahoo.com", "yahoo.co.uk",

  /* Other global domains */
  "email.com", "games.com" /* AOL */, "gmx.net", "hush.com", "hushmail.com", "inbox.com",
  "lavabit.com", "love.com" /* AOL */, "pobox.com", "rocketmail.com" /* Yahoo */,
  "safe-mail.net", "wow.com" /* AOL */, "ygm.com" /* AOL */, "ymail.com" /* Yahoo */, "zoho.com", "fastmail.fm",

  /* United States ISP domains */
  "bellsouth.net", "charter.net", "cox.net", "earthlink.net", "juno.com",

  /* British ISP domains */
  "btinternet.com", "virginmedia.com", "blueyonder.co.uk", "freeserve.co.uk", "live.co.uk",
  "ntlworld.com", "o2.co.uk", "orange.net", "sky.com", "talktalk.co.uk", "tiscali.co.uk",
  "virgin.net", "wanadoo.co.uk", "bt.com",

  /* Domains used in Asia */
  "sina.com", "qq.com", "naver.com", "hanmail.net", "daum.net", "nate.com", "yahoo.co.jp", "yahoo.co.kr", "yahoo.co.id", "yahoo.co.in", "yahoo.com.sg",

  /* French ISP domains */
  "hotmail.fr", "live.fr", "laposte.net", "yahoo.fr", "wanadoo.fr", "orange.fr", "gmx.fr", "sfr.fr", "neuf.fr", "free.fr",

  /* German ISP domains */
  "gmx.de", "hotmail.de", "live.de", "online.de", "t-online.de" /* T-Mobile */, "web.de", "yahoo.de",

  /* Russian ISP domains */
  "mail.ru", "rambler.ru", "yandex.ru",

  /* Belgian ISP domains */
  "hotmail.be", "live.be", "skynet.be", "voo.be", "tvcablenet.be",

  /* Argentinian ISP domains */
  "hotmail.com.ar", "live.com.ar", "yahoo.com.ar", "fibertel.com.ar", "speedy.com.ar", "arnet.com.ar",

  /* Domains used in Mexico */
  "hotmail.com", "gmail.com", "yahoo.com.mx", "live.com.mx", "yahoo.com", "hotmail.es", "live.com", "hotmail.com.mx", "prodigy.net.mx", "msn.com"
]
`

App.SignupView = Em.View.extend
  elementId: 'signup-view'

  passwordInputPattern: '.{6,}'
  passwordInputTitle: 'Minimum 6 characters'

  usernameFieldInvalid: Em.computed.equal('controller.invalidField', 'username')
  emailFieldInvalid:    Em.computed.equal('controller.invalidField', 'email')
  passwordFieldInvalid: Em.computed.equal('controller.invalidField', 'password')

  disableSubmitButton: (->
    !@get('controller.user.username') || !@get('controller.user.email') || !@get('controller.user.password') || @get('controller.signingUp')
  ).property 'controller.user.username', 'controller.user.email', 'controller.user.password', 'controller.signingUp'

  disableGitHubSignUp: (->
    @get('controller.signingUp') || @get('controller.signingUpWithGitHub')
  ).property 'controller.signingUp', 'controller.signingUpWithGitHub'

  focusInvalidField: (->
    if field = @get 'controller.invalidField'
      Em.run.schedule 'afterRender', this, ->
        @$("#signup-#{field}").select().focus()
  ).observes('controller.invalidField')

  checkEmail: (->
    Mailcheck.run(
      domains: domains
      topLevelDomains: ["co.jp", "co.uk", "com", "net", "org", "info", "edu", "gov", "mil", "ru", "de", "dk", "es", "fr", "gr", "ie", "in", "pl", "ro", "se", "ca", "cn", "co.id", "co.in", "co.kr", "co.nz", "fm", "io", "ua", "com.tr", "com.sg"]

      email: @get('controller.user.email')

      suggested: (s) =>
        if s.full.match(/^[^@]+@[^\.]+\.[^\.]+/)
          @set('controller.suggestedEmail', s.full)

      empty: (s) =>
        @set('controller.suggestedEmail', null)
    )
  ).observes('controller.user.email')

  didInsertElement: ->
    @_super.apply this, arguments
    Em.run.schedule 'afterRender', this, ->
      @$('#signup-username').select().focus()

  submitButtonLabel: (->
    if @get('controller.gitHubUserName')
      'Create New Account With GitHub'
    else
      'Create New Account'
  ).property('controller.gitHubUserName')
