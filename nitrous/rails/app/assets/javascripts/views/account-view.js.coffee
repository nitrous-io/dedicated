App.AccountView = Em.View.extend
  elementId: 'account-view'
  classNames: ['ui', 'card']

  emailFieldInvalid:    Em.computed.equal('controller.detailsInvalidField', 'email')
  passwordFieldInvalid: Em.computed.equal('controller.detailsInvalidField', 'password')

  didInsertElement: ->
    @_super()
    @$('[autofocus]').focus()
