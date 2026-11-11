import { Box, Text } from "@chakra-ui/react";
import { useConnectionContext } from "../connection";
import { DetailPage } from "../page/DetailPage";
import { PageHeader } from "../page/PageHeader";
import { PageIcon } from "../page/PageIcon";
import { SectionHeading } from "../page/SectionHeading";
import { Button } from "../ui/button/Button";
import { FullScreenLoader } from "../ui/loader/FullScreenLoader";
import { useTips, type FetchTips, type Tip } from "./tips";

function TipSection({ tip }: { tip: Tip }) {
  const id = `tip-${tip.id}`;
  return (
    <Box mt="6" role="region" aria-labelledby={id}>
      <SectionHeading id={id} tone="running">
        {tip.title}
      </SectionHeading>
      <Text whiteSpace="pre-line">{tip.body}</Text>
      {!!tip.folders?.length && (
        <Box as="ul" listStyleType="none" p="0" mt="2" wordBreak="break-all">
          {tip.folders.map((f) => (
            <Text as="li" key={f} color="text.muted" fontSize="sm">
              {f}
            </Text>
          ))}
        </Box>
      )}
    </Box>
  );
}

export function TipsPage({ fetchTips }: { fetchTips?: FetchTips }) {
  const conn = useConnectionContext();
  const { tips, failed, retry } = useTips(fetchTips, conn);
  return (
    <DetailPage title={{ kind: "Tips" }}>
      <Box mt="4">
        <PageHeader title="Tips" icon={<PageIcon kind="tips" />} />
      </Box>
      {failed ? (
        <Box mt="6" display="flex" alignItems="center" gap="3">
          <Text color="text.muted">Could not load tips</Text>
          <Button emphasis="ghost" onClick={retry}>
            Retry
          </Button>
        </Box>
      ) : !tips ? (
        <FullScreenLoader label="Loading tips" />
      ) : tips.length === 0 ? (
        <Text mt="6" color="text.muted">
          No tips for this machine
        </Text>
      ) : (
        tips.map((t) => <TipSection key={t.id} tip={t} />)
      )}
    </DetailPage>
  );
}
