App.UserSettingsRoute = App.AuthedRoute.extend
  activate: ->
    App.set 'title', 'Account Settings'

  renderTemplate: ->
    @render 'user-settings', outlet: 'content'

  setupController: (controller) ->
    @_super.apply this, arguments
    unless controller.get 'saving'
      controller.clear()
