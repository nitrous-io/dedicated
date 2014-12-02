App.IntroN2oRoute = Em.Route.extend
  model: ->
    App.get 'boxes.lastObject'

  setupController: (controller, model) ->
    @_super.apply this, arguments
    @controllerFor('n2oBonus').set 'isIntro', true

  renderTemplate: ->
    @_super()
    @render 'n2o/bonus', into: 'intro/n2o', outlet: 'n2oBonus', controller: 'n2oBonus'

  deactivate: ->
    @controller.set 'content', null
    @controllerFor('n2oBonus').set 'isIntro', false
