App.SliderView = Em.View.extend
  classNames: ['slider_view']
  classNameBindings: ['readOnly', 'percentageBelow50:below50', 'percentageBelow25:below25', 'percentageBelow0:below0']
  templateName: 'templates/components/slider'
  value: null
  min: 0
  max: 100
  step: 10
  unit: null
  readOnly: false

  init: ->
    unless value = @get 'value'
      @set 'value', @get('min')
    @_super.apply this, arguments

  valueLabel: (->
    "#{@get 'value'}#{@get('unit') || ''}"
  ).property 'value'

  percentage: (->
    value = @get 'value'
    max = @get 'max'
    min = @get 'min'
    (value - min) / (max - min) * 100
  ).property 'value', 'min', 'max'

  percentageBelow50: (->
    percentage = @get 'percentage'
    percentage >= 25 && percentage < 50
  ).property 'percentage'

  percentageBelow25: (->
    percentage = @get 'percentage'
    percentage >= 0 && percentage < 25
  ).property 'percentage'

  percentageBelow0: (->
    @get('percentage') < 0
  ).property 'percentage'

  didInsertElement: ->
    @_super()
    this._updateBar()

  mouseDown: (event) ->
    return if @get('readOnly')
    event.preventDefault()
    this.didMoveMouse event

    $body = $('body')

    $body.on 'mousemove.sliderView', (event) =>
      this.didMoveMouse event

    eventsOff = ->
      $body.off 'mousemove.sliderView'
      $body.off 'mouseup.sliderView'
      $body.off 'mouseleave.sliderView'

    $body.on 'mouseup.sliderView', eventsOff
    $body.on 'mouseleave.sliderView', eventsOff

  updateValueByFraction: (fraction) ->
    min = @get 'min'
    max = @get 'max'
    step = @get 'step'
    maxMinusMin = max - min

    valueMinusMin = Math.round(fraction * maxMinusMin / step) * step
    value = valueMinusMin + min
    @set 'value', value

  didMoveMouse: (event) ->
    $el = @$()
    width = $el.width()
    mouseX = Math.max(0, event.pageX - $el.offset().left)
    fraction = Math.min(1, (mouseX / width))
    this.updateValueByFraction fraction
    this._updateBar()

  _updateBar: (->
    @$('.bar')?.css('width', "#{Math.max(@get('percentage'), 0)}%")
  ).observes 'percentage'
