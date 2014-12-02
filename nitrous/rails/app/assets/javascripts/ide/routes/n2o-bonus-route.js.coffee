App.N2oBonusRoute = Em.Route.extend
  model: ->
    App.get 'bonus'

  setupController: (controller, model) ->
    @_super.apply this, arguments
    controller.set 'bonusInfo', App.get('bonusInfo')
    controller.set 'isIntro', false
    controller.reload()

  deactivate: ->
    @controller.setProperties(content: null, bonusInfo: null)
