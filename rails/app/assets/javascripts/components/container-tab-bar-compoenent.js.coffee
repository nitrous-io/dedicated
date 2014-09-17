App.ContainerTabBarComponent = Em.Component.extend
  tagName: 'ul'
  classNames: ['ui', 'tab-bar', 'container-tab-bar']

  selected: null

  generalSelected: Em.computed.equal 'selected', 'general'
  keysSelected:    Em.computed.equal 'selected', 'keys'
  configSelected:  Em.computed.equal 'selected', 'config'
  logsSelected:    Em.computed.equal 'selected', 'logs'

  didInsertElement: ->
    @_super()
    @$('li.disabled > a').on 'click.container-tab-bar', ->
      false

  willDestroyElement: ->
    @_super()
    @$('li.disabled > a').off 'click.container-tab-bar'
