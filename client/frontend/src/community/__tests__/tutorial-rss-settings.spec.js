import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import CommunitySettingsFields from '../CommunitySettingsFields.vue'
import {
  createEditionSettingsFields,
  getEditionServerFields,
  readEditionSettingsFields
} from '../settingsDependencies.js'

describe('Community public tutorial RSS setting', () => {
  it('starts blank, round-trips the saved URL, and leaves Agent settings out', async () => {
    expect(createEditionSettingsFields()).toEqual({ tutorialRSSURL: '' })
    expect(readEditionSettingsFields({ tutorialRSSURL: 'https://example.com/feed.xml' })).toEqual({
      tutorialRSSURL: 'https://example.com/feed.xml'
    })

    const fields = createEditionSettingsFields()
    const wrapper = mount(CommunitySettingsFields, { props: { section: 'service', fields } })
    await wrapper.get('input').setValue('https://example.com/feed.xml')
    expect(getEditionServerFields(fields)).toEqual({ tutorialRSSURL: 'https://example.com/feed.xml' })
    expect(wrapper.text()).toContain('公开教程')
  })
})
