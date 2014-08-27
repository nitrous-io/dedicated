App.HostsNewView = Em.View.extend
  elementId: 'hosts-new-view'
  classNames: ['ui', 'card']

  slugIsDirty: false

  nameFieldInvalid: Em.computed.equal('controller.invalidField', 'name')
  slugFieldInvalid: Em.computed.equal('controller.invalidField', 'slug')

  createButtonDisabled: (->
    !(@get('controller.name') && @get('controller.slug') && @get('controller.selectedRegionSlug') && @get('controller.selectedPlanSlug')) || @get('controller.saving')
  ).property 'controller.name', 'controller.slug', 'controller.selectedRegionSlug', 'controller.selectedPlanSlug', 'controller.saving'

  didInsertElement: ->
    @_super()
    controller = @get 'controller'
    $region = @$('input[name=region]')
    $plan = @$('input[name=size]')

    $region.on 'change.hosts-new', =>
      controller.set 'selectedRegionSlug', $region.filter(':checked').val()

    $plan.on 'change.hosts-new', =>
      controller.set 'selectedPlanSlug', $plan.filter(':checked').val()

    $region.eq(0).attr('checked', true).change()
    $plan.eq(1).attr('checked', true).change()

    @$('#hosts-new-name').on 'change.hosts-new keydown.hosts-new', (ev) =>
      Em.run.next =>
        unless @get('slugIsDirty')
          @set 'controller.slug', helpers.slugify($.trim(ev.target.value))

    @$('#hosts-new-slug').one 'change.hosts-new keydown.hosts-new', (ev) =>
      unless ev.target.value == helpers.slugify($.trim(@get 'controller.name'))
        @set 'slugIsDirty', true

    @$('#hosts-new-name').focus()

  willDestroyElement: ->
    @_super()
    @$('input[name=region]').off 'change.hosts-new'
    @$('input[name=size]').off 'change.hosts-new'
    @$('#hosts-new-name').off 'change.hosts-new keydown.hosts-new'
    @$('#hosts-new-slug').off 'change.hosts-new keydown.hosts-new'
