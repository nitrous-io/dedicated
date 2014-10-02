App.ImagesPullView = Em.View.extend
  elementId: 'images-pull-view'
  classNames: ['ui', 'card']

  hideProgress: Em.computed.not('controller.saving')
  indeterminateProgress: Em.computed.none('controller.image.progress.progressDetail.start')

  createButtonDisabled: (->
    !(@get('controller.selectedHostSlug') && @get('controller.repo'))
  ).property 'controller.selectedHostSlug', 'controller.repo'

  hostsDidChange: (->
    Em.run.scheduleOnce 'afterRender', this, ->
      slug = @get('controller.preselectHost')
      if slug && !@get('selectedHostSlug')
        @set 'controller.selectedHostSlug', slug
      @$('select[name=host]')?.change()
  ).observes 'App.hosts.@each'

  preselectHostDidChange: (->
    if slug = @get('controller.preselectHost')
      @set 'controller.selectedHostSlug', slug
  ).observes 'controller.preselectHost'
