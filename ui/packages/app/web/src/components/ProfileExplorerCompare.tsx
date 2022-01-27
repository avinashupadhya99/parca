import ProfileSelector, {QuerySelection} from './ProfileSelector';
import {ProfileDiffSource, ProfileSelection, ProfileView} from '@parca/profile';
import {Query} from '@parca/parser';
import {QueryServiceClient} from '@parca/client';

interface ProfileExplorerCompareProps {
  queryClient: QueryServiceClient;

  queryA: QuerySelection;
  queryB: QuerySelection;
  profileA: ProfileSelection | null;
  profileB: ProfileSelection | null;
  selectQueryA: (query: QuerySelection) => void;
  selectQueryB: (query: QuerySelection) => void;
  selectProfileA: (source: ProfileSelection) => void;
  selectProfileB: (source: ProfileSelection) => void;
  expandProfile: (query: QuerySelection) => void;
}

const ProfileExplorerCompare = ({
  queryClient,
  queryA,
  queryB,
  profileA,
  profileB,
  selectQueryA,
  selectQueryB,
  selectProfileA,
  selectProfileB,
  expandProfile,
}: ProfileExplorerCompareProps): JSX.Element => {
  return (
    <>
      <div className="grid grid-cols-2">
        <div className="pr-2">
          <ProfileSelector
            queryClient={queryClient}
            querySelection={queryA}
            profileSelection={profileA}
            selectProfile={selectProfileA}
            selectQuery={selectQueryA}
            expandProfile={expandProfile}
            enforcedProfileName={''}
            comparing={true}
            onCompareProfile={() => {}}
          />
        </div>
        <div className="pl-2">
          <ProfileSelector
            queryClient={queryClient}
            querySelection={queryB}
            profileSelection={profileB}
            selectProfile={selectProfileB}
            selectQuery={selectQueryB}
            expandProfile={expandProfile}
            enforcedProfileName={Query.parse(queryA.expression).profileName()}
            comparing={true}
            onCompareProfile={() => {}}
          />
        </div>
      </div>
      <div className="grid grid-cols-1">
        {profileA != null && profileB != null ? (
          <ProfileView
            queryClient={queryClient}
            profileSource={
              new ProfileDiffSource(profileA.ProfileSource(), profileB.ProfileSource())
            }
          />
        ) : (
          <div>
            <div className="my-20 text-center">
              <p>Select a profile on both sides.</p>
            </div>
          </div>
        )}
      </div>
    </>
  );
};

export default ProfileExplorerCompare;
