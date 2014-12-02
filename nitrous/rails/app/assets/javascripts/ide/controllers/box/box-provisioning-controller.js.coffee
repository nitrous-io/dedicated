App.BoxProvisioningController = Em.ObjectController.extend
  redirectBasedOnState: (->
    if content = @get 'content'
      if content.get('state') == 'running'
        Em.run.next ->
          if content.get('last_added_repository_id')
            App.get('router').transitionTo 'box.ide', content, queryParams: { repository_id: content.get('last_added_repository_id') }
          else
            App.get('router').transitionTo 'box.ide', content
      else if content.get('state') == 'failed'
        Em.run.next ->
          App.get('router').transitionTo 'boxes.index'
          App.set 'flashMessage', { type: 'error', text: 'Sorry, there seems to be a problem creating your box. Please try again later or in a different region.' }
  ).observes 'content.state'
