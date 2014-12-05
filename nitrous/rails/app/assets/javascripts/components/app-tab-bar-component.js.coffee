App.AppTabBarComponent = Em.Component.extend
  tagName: 'ul'
  classNames: ['ui', 'tab-bar', 'app-tab-bar']

  model: null
  selected: null

  generalSelected:   Em.computed.equal 'selected', 'general'
  metricsSelected:   Em.computed.equal 'selected', 'metrics'
  keysSelected:      Em.computed.equal 'selected', 'keys'
  snapshotsSelected: Em.computed.equal 'selected', 'snapshots'

  didInsertElement: ->
    @_super()
    @$('li.disabled > a').on 'click.app-tab-bar', ->
      false

  willDestroyElement: ->
    @_super()
    @$('li.disabled > a').off 'click.app-tab-bar'
