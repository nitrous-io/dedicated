App.ContainerTabBarComponent = Em.Component.extend
  tagName: 'ul'
  classNames: ['ui', 'tab-bar', 'container-tab-bar']

  selected: null

  generalSelected: Em.computed.equal 'selected', 'general'
  logsSelected:    Em.computed.equal 'selected', 'logs'
  keysSelected:    Em.computed.equal 'selected', 'keys'
  configSelected:  Em.computed.equal 'selected', 'config'
  usersSelected:   Em.computed.equal 'selected', 'users'

  didInsertElement: ->
    @_super()
    @$('li.disabled > a').on 'click.container-tab-bar', ->
      false

  willDestroyElement: ->
    @_super()
    @$('li.disabled > a').off 'click.container-tab-bar'
