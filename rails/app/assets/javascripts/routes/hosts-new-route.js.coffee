App.HostsNewRoute = Em.Route.extend
  activate: ->
    App.set 'title', 'New Host'

  renderTemplate: ->
    @render 'hosts-new', outlet: 'content'
