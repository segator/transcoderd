import base64
import copy
import unittest

from resolve_release import resolve


class ReleaseSourceTests(unittest.TestCase):
    def setUp(self):
        self.sha = 'a' * 40
        self.prefix = 'repos/owner/repo'
        self.responses = {
            '/releases/tags/v2.12.0': {
                'tag_name': 'v2.12.0', 'draft': False, 'prerelease': False, 'assets': []
            },
            '/git/ref/tags/v2.12.0': {'object': {'type': 'commit', 'sha': self.sha}},
            f'/compare/{self.sha}...main': {'status': 'ahead'},
            f'/contents/version.txt?ref={self.sha}': {
                'encoding': 'base64', 'content': base64.b64encode(b'2.12.0\n').decode()
            },
        }

    def api(self, path):
        self.assertTrue(path.startswith(self.prefix))
        return copy.deepcopy(self.responses[path[len(self.prefix):]])

    def resolve(self, **changes):
        args = dict(tag='v2.12.0', event='workflow_dispatch', ref='refs/heads/main',
                    sha='b' * 40, debug_run='', repository='owner/repo', api=self.api)
        args.update(changes)
        return resolve(**args)

    def test_exact_release_source_not_workflow_source(self):
        self.assertEqual(self.resolve(), dict(source_ref=self.sha, source_sha=self.sha,
                                              release_tag='v2.12.0', image_target='docker:all'))

    def test_normal_checkout_preserved_without_api_calls(self):
        self.assertEqual(self.resolve(tag='', api=lambda _: self.fail('Unexpected API call')),
                         dict(source_ref='', source_sha='b' * 40, release_tag='',
                              image_target='docker:check'))

    def test_untrusted_dispatch_and_injected_tags(self):
        for changes in [dict(event='pull_request'), dict(ref='refs/heads/other'),
                        dict(debug_run='123'), dict(tag='v2.12.0-rc.1'),
                        dict(tag='v02.12.0'), dict(tag='v2.12.0\nsource_ref=evil')]:
            with self.subTest(changes=changes), self.assertRaises(ValueError):
                self.resolve(**changes)

    def test_unpublished_and_signed_releases_rejected(self):
        path = '/releases/tags/v2.12.0'
        for mutation in [dict(draft=True), dict(prerelease=True), dict(tag_name='v2.11.0'),
                         dict(assets=[dict(name='windows-update.json.sig')])]:
            original = copy.deepcopy(self.responses[path])
            self.responses[path].update(mutation)
            with self.subTest(mutation=mutation), self.assertRaises(ValueError):
                self.resolve()
            self.responses[path] = original

    def test_off_mainline_rejected(self):
        for status in ['behind', 'diverged']:
            self.responses[f'/compare/{self.sha}...main']['status'] = status
            with self.subTest(status=status), self.assertRaises(ValueError):
                self.resolve()

    def test_version_mismatch_rejected(self):
        self.responses[f'/contents/version.txt?ref={self.sha}']['content'] = 'Mi4xMS4w'
        with self.assertRaises(ValueError):
            self.resolve()

    def test_annotated_tag_and_cycle(self):
        annotation = 'c' * 40
        self.responses['/git/ref/tags/v2.12.0']['object'] = dict(type='tag', sha=annotation)
        self.responses[f'/git/tags/{annotation}'] = {'object': dict(type='commit', sha=self.sha)}
        self.assertEqual(self.resolve()['source_sha'], self.sha)
        self.responses[f'/git/tags/{annotation}'] = {'object': dict(type='tag', sha=annotation)}
        with self.assertRaises(ValueError):
            self.resolve()


if __name__ == '__main__':
    unittest.main()
