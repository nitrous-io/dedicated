class PagesController < ApplicationController
  # rails is used only for asset precompilation
  # keep actions empty

  ALLOWED_JOBS = %w(software-engineer platform-engineer developer-evangelist support-engineer)

  def home
  end

  def features
  end

  def pricing
  end

  def company
  end

  def stacks
  end

  def stack
  end

  def whatsnew
  end

  def plan_changes
  end

  def education_pricing
  end

  def teams_pricing
  end

  def jobs
  end

  def job
    if ALLOWED_JOBS.include? params[:job]
      render "pages/jobs/#{params[:job].to_s}"
    else
      render status: :not_found
    end
  end
end
