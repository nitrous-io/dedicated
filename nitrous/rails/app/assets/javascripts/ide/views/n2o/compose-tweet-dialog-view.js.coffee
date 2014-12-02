App.ComposeTweetDialogView = App.IdeModalDialogView.extend
  templateName: 'templates/n2o/compose-tweet-dialog'
  elementId: 'compose_tweet_dialog_view'
  contentBinding: 'controller.composeTweetDialog' # N2oBonusController

  message: null

  didInsertElement: ->
    @set 'message', "I can code Ruby/Python/Node.JS & Go apps on Nitrous! Sign Up now and get 250MB free space: #{App.get 'currentUser.referralLinkFullTwitterURL'}"

  messageInvalid: (->
    messageLength = @get 'messageLength'
    !(messageLength > 0 && messageLength <= 140)
  ).property 'message'

  messageLength: (->
    unless message = @get('message')
      return 0

    return message.length
  ).property 'message'

  didClickSendTweet: ->
    unless @get 'messageInvalid'
      @get('controller').sendTweet @get('message')
      this.hide()

App.ComposeTweetDialogTextArea = Em.TextArea.extend
  insertNewline: (event) ->
    @get('parentView').didClickSendTweet()
