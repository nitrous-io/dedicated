App.HostVolumesNewView = Em.View.extend
  elementId: 'host-volumes-new-view'

  slugIsDirty: false

  createButtonDisabled: (->
    !(@get('controller.name') && @get('controller.slug'))
  ).property 'controller.name', 'controller.slug'

  didInsertElement: ->
    @_super()

    @$('#volumes-new-name').on 'change.volumes-new keydown.volumes-new', (ev) =>
      Em.run.next =>
        unless @get('slugIsDirty')
          @set 'controller.slug', helpers.slugify($.trim(ev.target.value))

    @$('#volumes-new-slug').one 'change.volumes-new keydown.volumes-new', (ev) =>
      unless ev.target.value == helpers.slugify($.trim(@get 'controller.name'))
        @set 'slugIsDirty', true

  willDestroyElement: ->
    @_super()
    @$('#volumes-new-name').off 'change.volumes-new keydown.volumes-new'
    @$('#volumes-new-slug').off 'change.volumes-new keydown.volumes-new'
