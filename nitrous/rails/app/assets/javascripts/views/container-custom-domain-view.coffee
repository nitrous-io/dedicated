App.ContainerCustomDomainView = Em.View.extend
  elementId: 'container-custom-domain-view'
  modalSelector: '#confirm-enable-hosting-mode-modal-dialog'

  actions:
    enableHostingMode: ->
      ui.showModal(@get('modalSelector'))

    confirmHostingMode: ->
      ui.hideModal(@get('modalSelector'))
      @get('controller').send('enableHostingMode')

    cancel: ->
      ui.hideModal(@get('modalSelector'))

    contactSupport: ->
      App.trigger 'open-feedback-dialog', 'Contact Support'

    goToPortsTab: ->
      @get('controller').transitionToRoute('container.ports')
