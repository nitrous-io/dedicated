App.BoxDomainsController = Em.ObjectController.extend
  boxCustomDomainPlaceholder: 'your.domain.com'

  save: ->
    box = @get 'content'
    box.updateCustomDomain (err) ->
      return if err
      App.get('router').transitionTo 'boxes.index'
      Em.run.next ->
        if box.get('custom_domain')?
          App.set 'flashMessage', { type: 'alert-info', text: 'Your box will shortly be accessible for preview at the new domain.' }
        else
          App.set 'flashMessage', { type: 'alert-info', text: 'Custom domain for this box has been removed' }

  cancel: ->
    App.get('router').transitionTo 'boxes.index'
