#= require ./admin-route

App.AdminUserRoute = App.AdminRoute.extend
  model: (params) ->
    p = App.AdminUser.fetchUsers(params.slug)
    p.then (model)->
      App.set 'title', "Admin - #{model.get('firstObject.username')}"
    , ()=>
      @transitionTo 'home'
    p

  renderTemplate: ->
    @render 'admin/user', outlet: 'content'