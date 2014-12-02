App.PartOutputStream = Em.ArrayProxy.extend
  content: []
  completed: false
  exitStatus: null
  onComplete: null

  overrideLine: false

  reset: ->
    @set 'content', []
    @set 'completed', false

  _didCompleteSuccessfully: (->
    if @get('completed') && @get('exitStatus') == 0
      @get('onComplete')?()
  ).observes 'completed'

  noCarriageReturn: (data) ->
    data.indexOf('\n') == -1

  write: (data) ->
    # If a line ends without a carriage return,
    # set the override mode.
    # If override mode is already set,
    # override the last line.
    if this.noCarriageReturn(data)
      if @get('overrideLine')
        @popObject()
      @set 'overrideLine', true
    else
      @set 'overrideLine', false
    @pushObject data
