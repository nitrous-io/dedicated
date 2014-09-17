App.HostsNewView = Em.View.extend
  elementId: 'hosts-new-view'
  classNames: ['ui', 'card']

  nameFieldInvalid: Em.computed.equal('controller.invalidField', 'name')
  slugFieldInvalid: Em.computed.equal('controller.invalidField', 'slug')

  createButtonDisabled: (->
    !(@get('controller.name') && @get('controller.slug') && @get('controller.selectedRegionSlug') && @get('controller.selectedPlanSlug')) || @get('controller.saving')
  ).property 'controller.name', 'controller.slug', 'controller.selectedRegionSlug', 'controller.selectedPlanSlug', 'controller.saving'

  preselectRegion: (->
    Em.run.scheduleOnce 'afterRender', this, ->
      @$('input[name=region]')?.eq(0).attr('checked', true).change()
  ).observes 'App.regions.@each'

  preselectPlan: (->
    Em.run.scheduleOnce 'afterRender', this, ->
      @$('input[name=size]')?.eq(1).attr('checked', true).change()
  ).observes 'App.plans.@each'

  didInsertElement: ->
    @_super()
    controller = @get 'controller'

    @$().on 'change.hosts-new', 'input[name=region]', =>
      controller.set 'selectedRegionSlug', @$('input[name=region]').filter(':checked').val()

    @$().on 'change.hosts-new', 'input[name=size]', =>
      controller.set 'selectedPlanSlug', @$('input[name=size]').filter(':checked').val()

    @preselectRegion()
    @preselectPlan()

    App.slug.slugify(@$('#hosts-new-name'), @$('#hosts-new-slug'))

  willDestroyElement: ->
    @_super()
    App.slug.unslugify(@$('#hosts-new-name'))
    @$().off 'change.hosts-new'
