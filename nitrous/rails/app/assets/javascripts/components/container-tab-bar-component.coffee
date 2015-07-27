App.ContainerTabBarComponent = Em.Component.extend
  tagName: 'ul'
  classNames: ['ui', 'tab-bar', 'container-tab-bar']

  model: null
  selected: null

  generalSelected:       Em.computed.equal 'selected', 'general'
  keysSelected:          Em.computed.equal 'selected', 'keys'
  portsSelected:         Em.computed.equal 'selected', 'ports'
  configSelected:        Em.computed.equal 'selected', 'config'
  collaboratorsSelected: Em.computed.equal 'selected', 'collaborators'
  customDomainSelected:  Em.computed.equal 'selected', 'custom-domain'

  didInsertElement: ->
    @_super()
    @$('li.disabled > a').on 'click.container-tab-bar', ->
      false

  willDestroyElement: ->
    @_super()
    @$('li.disabled > a').off 'click.container-tab-bar'
