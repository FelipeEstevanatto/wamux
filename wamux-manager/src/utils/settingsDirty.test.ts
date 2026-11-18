import { describe, expect, it } from 'vitest';

import { proxyFormDirty, sameMembers } from './settingsDirty';

describe('sameMembers', () => {
  it('ignores order', () => {
    expect(sameMembers(['MESSAGE', 'CALL'], ['CALL', 'MESSAGE'])).toBe(true);
    expect(sameMembers(['MESSAGE'], ['MESSAGE', 'CALL'])).toBe(false);
  });
});

describe('proxyFormDirty', () => {
  const saved = { protocol: 'http', host: '10.0.0.1', port: '8080', username: 'ana' };

  it('is clean when the form matches the saved proxy and the password is blank', () => {
    expect(proxyFormDirty({ ...saved, password: '' }, saved)).toBe(false);
    expect(proxyFormDirty({ protocol: 'http', host: '', port: '', username: '', password: '' }, null)).toBe(
      false
    );
  });

  it('is dirty when a field or the password changes', () => {
    expect(proxyFormDirty({ ...saved, port: '9090', password: '' }, saved)).toBe(true);
    expect(proxyFormDirty({ ...saved, password: 'secret' }, saved)).toBe(true);
  });
});
