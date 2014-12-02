App.InvitesView = Em.View.extend
  templateName: 'templates/invites'
  classNames: ['app_page', 'invites', 'index']
  elementId: 'invites-view'
  emailFieldText: null

  didInsertElement: ->
    @_super()
    App.util.mixpanel 'Viewed Invites Page'

  didClickInvite: ->
    @get('controller')?.sendInvite()

App.NewInviteTextField = Em.TextField.extend
  attributeBindings: ['placeholder']
  placeholder: 'hello@nitrous.io'
  classNames: ['mailcheck']

  insertNewline: ->
    @get('parentView')?.didClickInvite()

App.InvitesReferralLinkTextField = Em.TextField.extend
  click: ->
    @$().select()

App.TwitterShareButtonView = Em.View.extend
  tagName: 'a'
  classNames: ['twitter-share-button']
  attributeBindings: ['href', 'url:data-url', 'text:data-text']

  href: "https://twitter.com/share"
  text: "Spin up your own dev environment to code in the cloud now! Get 250MB of additional space by using this link:"

  didInsertElement: ->
    Shareable.initTwitterButton document, 'script', 'twitter-wjs'

App.FacebookShareButtonView = Em.View.extend
  tagName: 'div'
  classNames: ['fb-like']
  attributeBindings: ['url:data-href', 'send:data-send', 'buttonLayout:data-layout', 'width:data-width', 'showFaces:data-show-faces', 'action:data-action']

  send: 'false'
  buttonLayout: 'button_count'
  width: '450'
  showFaces: 'false'
  action: 'like'

  didInsertElement: ->
    Shareable.initFacebookButton document, 'script', 'facebook-jssdk'

App.GooglePlusShareButtonView = Em.View.extend
  tagName: 'div'
  classNames: ['g-plus']
  attributeBindings: ['action:data-action', 'annotation:data-annotation', 'url:data-href']

  action: "share"
  annotation: "bubble"

  didInsertElement: ->
    Shareable.initGooglePlusButton()
