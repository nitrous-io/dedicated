App.ReceiptsView = Em.View.extend
  templateName: 'templates/account/receipts'
  elementId: 'receipts'

  didInsertElement: ->
    @get('controller').fetchReceipts()
