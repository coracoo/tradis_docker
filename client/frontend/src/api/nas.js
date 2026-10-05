import { get } from '../utils/request.js'

export const getNASCategories = (options = {}) => {
  return get('/nas/categories', {}, options)
}

export const getNASDevices = (params = {}, options = {}) => {
  return get('/nas/list', params, options)
}

export const getNASNetworkTiers = (options = {}) => {
  return get('/nas/network-tiers', {}, options)
}

export const getNASPromotionTags = (options = {}) => {
  return get('/nas/tags', {}, options)
}

export const getNASBanners = (options = {}) => {
  return get('/nas/banners', {}, options)
}

export const getNASReviews = (params = {}, options = {}) => {
  return get('/nas/reviews', params, options)
}

export const getNASReview = (id, options = {}) => {
  return get(`/nas/reviews/${id}`, {}, options)
}

export const getNASTopics = (options = {}) => {
  return get('/nas/topics', {}, options)
}

export const getNASTopic = (slug, options = {}) => {
  return get(`/nas/topics/${slug}`, {}, options)
}
