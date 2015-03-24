class PagesController < ApplicationController
  def home
  end

  def pricing
  end
    
  def stack        
    slug = params[:stack].presence || 'ruby'
    stacks = YAML.load(File.read(File.join(Rails.root, 'config', 'stacks.yml'))) 
    if !stacks[slug]
      render nothing: true, status: 404
      return
    end
    # TrackMixpanelEventJob.submit_job('Visited Stack Page', current_user.try(:id), stack: slug)
    @stacks = stacks.to_a
    @stack = stacks[slug]
  end
end
