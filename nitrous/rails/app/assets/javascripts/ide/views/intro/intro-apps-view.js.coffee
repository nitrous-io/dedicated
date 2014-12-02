App.IntroAppsView = Em.View.extend
  templateName: 'templates/intro/apps'
  elementId: 'intro_apps_view'
  classNames: ['app_page', 'intro']

  didInsertElement: ->
    @_super()
    App.util.mixpanel 'Viewed Intro Desktop Apps Page'

  didClickNextScreen: ->
    box = @get 'controller.content'
    state = box.get 'state'

    switch state
      when 'running'
        if lastAddedRepositoryId = box.get('last_added_repository_id')
          App.get('router').transitionTo 'box.ide', box, queryParams: { repository_id: lastAddedRepositoryId }
        else
          App.get('router').transitionTo 'box.ide', box
      when 'provisioning'
        box.set 'expanded', true
        App.get('router').transitionTo 'boxes.index'
      when 'failed'
        box.set 'expanded', true
        App.get('router').transitionTo 'boxes.index'
        Em.run.next ->
          App.set 'flashMessage', { type: 'error', text: 'Sorry, there seems to be a problem creating your box. Please try again later or in a different region.' }
      else
        App.get('router').transitionTo 'boxes.index'
