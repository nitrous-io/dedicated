App.ContainerTabBarComponent = Em.Component.extend
  tagName: 'ul'
  classNames: ['ui', 'tab-bar', 'container-tab-bar']

  selected: null

  generalSelected: Em.computed.equal 'selected', 'general'
  keysSelected:    Em.computed.equal 'selected', 'keys'
  volumesSelected: Em.computed.equal 'selected', 'volumes'
  portsSelected:   Em.computed.equal 'selected', 'ports'
  envSelected:     Em.computed.equal 'selected', 'env'
  logsSelected:    Em.computed.equal 'selected', 'logs'
  imagesSelected:  Em.computed.equal 'selected', 'images'
  usersSelected:   Em.computed.equal 'selected', 'users'

  didInsertElement: ->
    @_super()
    @$('li.disabled > a').on 'click.container-tab-bar', ->
      false

  willDestroyElement: ->
    @_super()
    @$('li.disabled > a').off 'click.container-tab-bar'
