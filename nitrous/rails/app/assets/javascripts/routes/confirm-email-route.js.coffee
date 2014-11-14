App.ConfirmEmailRoute = Em.Route.extend
  beforeModel: ->
    @_super.apply this, arguments

  activate: ->
    App.set 'title', 'Confirm Account'

  renderTemplate: ->
    @_super()
    @render 'topnav', into: 'confirm-email', outlet: 'topnav', controller: 'topnav'

  setupController: (controller) ->
    controller.startConfirm()
