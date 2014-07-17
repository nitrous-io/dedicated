App.HostTabBarComponent = Em.Component.extend
  tagName: 'ul'
  classNames: ['ui', 'tab-bar', 'host-tab-bar']

  selected: null

  generalSelected:    Em.computed.equal 'selected', 'general'
  registriesSelected: Em.computed.equal 'selected', 'registries'
  imagesSelected:     Em.computed.equal 'selected', 'images'
  archivesSelected:   Em.computed.equal 'selected', 'archives'
  volumesSelected:    Em.computed.equal 'selected', 'volumes'

  didInsertElement: ->
    @_super()
    @$('li.disabled > a').on 'click.host-tab-bar', ->
      false

  willDestroyElement: ->
    @_super()
    @$('li.disabled > a').off 'click.host-tab-bar'
