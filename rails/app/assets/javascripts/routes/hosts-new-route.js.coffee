App.HostsNewRoute = App.AuthedRoute.extend
  activate: ->
    App.set 'title', 'New Host'

  renderTemplate: ->
    @render 'hosts/new', outlet: 'content'
