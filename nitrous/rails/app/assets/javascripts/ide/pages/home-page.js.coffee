window.App ||= {}

class App.HomePage
  constructor: (email, mixpanelProperties = {}) ->
    if mixpanel?
      mixpanel.register(mixpanelProperties)
      mixpanel.identify email if email?.length > 0
      mixpanel.track('Viewed Home Page', mixpanelProperties)
    $('#first-step').focus()

    @stacks = ["Ruby", "Python", "Node.js", "PHP", "Go", "Meteor"]
    @i = 0

    this.changeStackName()
    setInterval(=>
      this.changeStackName()
    , 3000)

    $emailConfirmation = $('#email-confirmation')
    if $emailConfirmation.length > 0
      $('.close-button', $emailConfirmation).click ->
        $emailConfirmation.hide()

  changeStackName: ->
    @$stackNameSpan ?= $("#stack-name span")
    @$stackNameSpan.text(@stacks[@i % @stacks.length])
    @i += 1
